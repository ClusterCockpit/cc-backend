// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// ErrNoConfig is returned by ConfigStore.Resolve when no configuration file
// applies to the requested member — not even a global default. Callers should
// treat this as "nothing to deploy", distinct from a malformed tree.
var ErrNoConfig = errors.New("fleet: no configuration found")

// errTreeChanged is an internal, transient signal that a file changed while it
// was being read during a scan. The scan is discarded and retried; the current
// snapshot keeps serving in the meantime.
var errTreeChanged = errors.New("fleet: config tree changed during scan")

// ConfigStore serves central configuration to fleet members from a
// hand-edited, hierarchical tree of JSON files on disk. Humans author the tree
// directly (editing redundant options in UI forms was rejected as too
// tedious); because options are shared, config is resolved by deep-merging
// from broad to specific so a value set once high in the tree is inherited and
// only overridden where it differs.
//
// Tree layout (missing layers are skipped, never an error):
//
//	<root>/
//	  defaults.json                     global defaults, every member
//	  <service_type>/
//	    defaults.json                   per service_type defaults
//	    <cluster>/defaults.json         cluster-scope only: per-cluster defaults
//	    <cluster>/<hostname>.json       cluster-scope leaf
//	    <hostname>.json                 infra-scope leaf
//
// Merge order (later overrides earlier, objects merge recursively, scalars and
// arrays overwrite wholesale):
//
//	cluster: global -> type -> cluster-defaults -> cluster/host
//	infra:   global -> type -> host
//
// # Snapshot swap
//
// Pulls never read the filesystem. A loader parses the entire tree into an
// immutable in-memory snapshot and publishes it via an atomic pointer swap;
// Resolve reads only the currently-published snapshot. This gives two
// guarantees the request path could not otherwise make while the tree is being
// hand-edited:
//
//   - No torn reads. Each file is read whole and parsed by the loader; a file
//     caught mid-write fails to parse (or its size/mtime changes across the
//     read), so that scan is discarded and the previous good snapshot keeps
//     serving. A partial or invalid tree is never published.
//   - Consistency and last-known-good. A generation is an all-or-nothing view
//     of the tree; a broken edit does not break running services — they keep
//     the last successfully-loaded config until the tree parses cleanly again.
//
// Freshness comes from Start (periodic reload) or an explicit Reload; the hot
// path stays allocation-light and lock-free on the snapshot pointer.
//
// The resolved config's revision is a content hash (fnv-1a 64-bit), so it
// changes exactly when the merged result changes and needs no manual counter
// kept in sync with the files. It fits the config_revision INTEGER column and
// the *Registry.AckConfig(int64) handshake.
type ConfigStore struct {
	root string

	snap atomic.Pointer[snapshot] // currently published generation (nil until first load)

	vmu        sync.RWMutex
	validators map[string]func(json.RawMessage) error
}

// snapshot is an immutable, fully-parsed view of the config tree. Once
// published it is never mutated; a reload builds a fresh one and swaps it in.
type snapshot struct {
	generation uint64
	signature  string                    // path:mtime:size of every file, for change detection
	files      map[string]map[string]any // relpath -> parsed JSON object
	cache      sync.Map                  // memberKey -> resolved (lazily filled, generation-scoped)
}

type resolved struct {
	blob     json.RawMessage
	revision int64
}

// NewConfigStore returns a ConfigStore rooted at dir. No filesystem access
// happens until the first Reload/Start or the first Resolve (which lazily
// loads once).
func NewConfigStore(dir string) *ConfigStore {
	return &ConfigStore{
		root:       dir,
		validators: make(map[string]func(json.RawMessage) error),
	}
}

// SetValidator registers an optional validator for a service_type. When set,
// the merged config is validated before it is served; a validation error fails
// the resolve rather than shipping broken config. Intended to be called during
// initialization, before Start/Resolve.
func (c *ConfigStore) SetValidator(serviceType string, fn func(json.RawMessage) error) {
	c.vmu.Lock()
	defer c.vmu.Unlock()
	c.validators[serviceType] = fn
}

// Start loads the tree once synchronously, then reloads it every interval until
// ctx is cancelled. A failed reload (torn write, malformed file) is logged and
// the previous snapshot keeps serving. Mirrors Registry.StartSweep's lifecycle.
func (c *ConfigStore) Start(ctx context.Context, interval time.Duration) {
	if err := c.Reload(); err != nil {
		cclog.Errorf("fleet: initial config load failed: %v", err)
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.Reload(); err != nil {
					cclog.Errorf("fleet: config reload failed, keeping previous snapshot: %v", err)
				}
			}
		}
	}()
}

// Reload rescans the tree and, if it changed and parses cleanly, atomically
// publishes it as the new snapshot. It is a no-op when the tree is unchanged.
// On any error the currently-published snapshot is left untouched.
func (c *ConfigStore) Reload() error {
	files, signature, err := c.scan()
	if err != nil {
		return err
	}
	cur := c.snap.Load()
	if cur != nil && cur.signature == signature {
		return nil
	}
	var gen uint64 = 1
	if cur != nil {
		gen = cur.generation + 1
	}
	c.snap.Store(&snapshot{generation: gen, signature: signature, files: files})
	return nil
}

// Resolve merges the applicable layers for a fleet member and returns the
// merged configuration plus its content-hash revision. scope must be
// ScopeCluster or ScopeInfra; cluster is ignored (and may be empty) for infra.
func (c *ConfigStore) Resolve(scope, cluster, serviceType, hostname string) (json.RawMessage, int64, error) {
	if err := validComponent(serviceType); err != nil {
		return nil, 0, fmt.Errorf("fleet: invalid service_type: %w", err)
	}
	if err := validComponent(hostname); err != nil {
		return nil, 0, fmt.Errorf("fleet: invalid hostname: %w", err)
	}

	rels, err := c.layerRelPaths(scope, cluster, serviceType, hostname)
	if err != nil {
		return nil, 0, err
	}

	snap := c.snap.Load()
	if snap == nil {
		// Lazy first load for callers that never called Start (tests, or a
		// pull racing startup).
		if err := c.Reload(); err != nil {
			return nil, 0, err
		}
		snap = c.snap.Load()
	}

	key := scope + "|" + cluster + "|" + serviceType + "|" + hostname
	if v, ok := snap.cache.Load(key); ok {
		r := v.(resolved)
		return cloneRaw(r.blob), r.revision, nil
	}

	merged := make(map[string]any)
	found := false
	for _, rel := range rels {
		obj, ok := snap.files[rel]
		if !ok {
			continue
		}
		deepMerge(merged, obj) // deepMerge copies nested maps; snap.files stays immutable
		found = true
	}
	if !found {
		return nil, 0, ErrNoConfig
	}

	// json.Marshal sorts map keys, so the encoding — and therefore the
	// revision — is stable across resolves of an unchanged tree.
	blob, merr := json.Marshal(merged)
	if merr != nil {
		return nil, 0, merr
	}

	c.vmu.RLock()
	validate := c.validators[serviceType]
	c.vmu.RUnlock()
	if validate != nil {
		if verr := validate(blob); verr != nil {
			return nil, 0, fmt.Errorf("fleet: config for service_type %q failed validation: %w", serviceType, verr)
		}
	}

	h := fnv.New64a()
	_, _ = h.Write(blob)
	revision := int64(h.Sum64() >> 1) // >>1 keeps it non-negative

	snap.cache.Store(key, resolved{blob: blob, revision: revision})
	return cloneRaw(blob), revision, nil
}

// scan walks the tree, reads and parses every *.json file, and returns the
// parsed files keyed by path relative to root plus a signature of their
// mtimes/sizes. It refuses to return a partial view: a file that fails to parse
// is an error, and a file whose size/mtime changes across its own read yields
// errTreeChanged so the caller can retry rather than publish a torn generation.
func (c *ConfigStore) scan() (map[string]map[string]any, string, error) {
	if _, err := os.Stat(c.root); errors.Is(err, fs.ErrNotExist) {
		// An absent tree is a valid empty configuration, not an error.
		return map[string]map[string]any{}, "", nil
	}

	var paths []string
	walkErr := filepath.WalkDir(c.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".json") {
			paths = append(paths, p)
		}
		return nil
	})
	if walkErr != nil {
		return nil, "", walkErr
	}
	sort.Strings(paths) // deterministic signature/merge order

	files := make(map[string]map[string]any, len(paths))
	var sig strings.Builder
	for _, p := range paths {
		fi1, err := os.Stat(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, "", errTreeChanged // removed mid-scan
			}
			return nil, "", err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, "", err
		}
		fi2, err := os.Stat(p)
		if err != nil {
			return nil, "", err
		}
		// Detect a write that overlapped this file's read.
		if fi1.Size() != fi2.Size() || !fi1.ModTime().Equal(fi2.ModTime()) {
			return nil, "", errTreeChanged
		}

		obj := make(map[string]any)
		// Unmarshalling into a map also enforces that every layer is a JSON
		// object; arrays or scalars at the top level are a config error.
		rel, relErr := filepath.Rel(c.root, p)
		if relErr != nil {
			return nil, "", relErr
		}
		if uerr := json.Unmarshal(raw, &obj); uerr != nil {
			return nil, "", fmt.Errorf("fleet: invalid config %s: %w", rel, uerr)
		}
		files[rel] = obj
		fmt.Fprintf(&sig, "%s:%d:%d;", rel, fi2.ModTime().UnixNano(), fi2.Size())
	}
	return files, sig.String(), nil
}

// layerRelPaths returns the ordered list of candidate layer files (broad ->
// specific) for a member, as paths relative to root — the keys into a
// snapshot's files map.
func (c *ConfigStore) layerRelPaths(scope, cluster, serviceType, hostname string) ([]string, error) {
	switch scope {
	case ScopeCluster:
		if err := validComponent(cluster); err != nil {
			return nil, fmt.Errorf("fleet: invalid cluster: %w", err)
		}
		return []string{
			"defaults.json",
			filepath.Join(serviceType, "defaults.json"),
			filepath.Join(serviceType, cluster, "defaults.json"),
			filepath.Join(serviceType, cluster, hostname+".json"),
		}, nil
	case ScopeInfra:
		return []string{
			"defaults.json",
			filepath.Join(serviceType, "defaults.json"),
			filepath.Join(serviceType, hostname+".json"),
		}, nil
	default:
		return nil, fmt.Errorf("fleet: unknown scope %q", scope)
	}
}

// deepMerge overlays src onto dst: nested objects merge recursively; every
// other value (scalars, arrays) overwrites wholesale. src is never mutated —
// nested source maps are deep-copied — so a shared, immutable snapshot map can
// be passed as src safely.
func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			if dv, ok := dst[k].(map[string]any); ok {
				deepMerge(dv, sv)
				continue
			}
			nv := make(map[string]any, len(sv))
			deepMerge(nv, sv)
			dst[k] = nv
			continue
		}
		dst[k] = v
	}
}

// validComponent rejects path components that are empty or could escape the
// config root (path separators, "." / ".."). service_type/cluster/hostname all
// become directory or file names, so they must be safe.
func validComponent(s string) error {
	if s == "" {
		return errors.New("empty")
	}
	if s == "." || s == ".." {
		return errors.New("must not be '.' or '..'")
	}
	if strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") {
		return errors.New("must not contain path separators or '..'")
	}
	return nil
}

func cloneRaw(b json.RawMessage) json.RawMessage {
	out := make(json.RawMessage, len(b))
	copy(out, b)
	return out
}
