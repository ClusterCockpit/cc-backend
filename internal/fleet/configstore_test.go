// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fleet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigStoreResolve(t *testing.T) {
	root := t.TempDir()
	// global defaults, per-type override + new key, infra host leaf override.
	writeFile(t, filepath.Join(root, "defaults.json"), `{"interval":10,"log":{"level":"info","file":"/var/log/a"}}`)
	writeFile(t, filepath.Join(root, "metric-store", "defaults.json"), `{"interval":30,"retention":7}`)
	writeFile(t, filepath.Join(root, "metric-store", "ms01.json"), `{"log":{"level":"debug"}}`)

	cs := NewConfigStore(root)
	blob, rev, err := cs.Resolve(ScopeInfra, "", "metric-store", "ms01")
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatal(err)
	}

	// interval: type(30) overrides global(10); retention only from type;
	// log deep-merges: level from leaf(debug), file preserved from global.
	if got["interval"].(float64) != 30 {
		t.Errorf("interval = %v, want 30", got["interval"])
	}
	if got["retention"].(float64) != 7 {
		t.Errorf("retention = %v, want 7", got["retention"])
	}
	log := got["log"].(map[string]any)
	if log["level"] != "debug" {
		t.Errorf("log.level = %v, want debug", log["level"])
	}
	if log["file"] != "/var/log/a" {
		t.Errorf("log.file = %v, want /var/log/a (inherited)", log["file"])
	}
	if rev == 0 {
		t.Error("revision should be non-zero")
	}

	// Stable across resolves of an unchanged tree (served from cache).
	_, rev2, err := cs.Resolve(ScopeInfra, "", "metric-store", "ms01")
	if err != nil {
		t.Fatal(err)
	}
	if rev2 != rev {
		t.Errorf("revision changed without edit: %d -> %d", rev, rev2)
	}
}

func TestConfigStoreClusterScope(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "defaults.json"), `{"a":1}`)
	writeFile(t, filepath.Join(root, "agent", "fritz", "defaults.json"), `{"a":2,"b":2}`)
	writeFile(t, filepath.Join(root, "agent", "fritz", "node01.json"), `{"b":3}`)

	cs := NewConfigStore(root)
	blob, _, err := cs.Resolve(ScopeCluster, "fritz", "agent", "node01")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatal(err)
	}
	if got["a"].(float64) != 2 || got["b"].(float64) != 3 {
		t.Fatalf("unexpected merge result: %v", got)
	}
}

func TestConfigStoreRevisionChangesOnEdit(t *testing.T) {
	root := t.TempDir()
	leaf := filepath.Join(root, "metric-store", "ms01.json")
	writeFile(t, filepath.Join(root, "defaults.json"), `{"x":1}`)
	writeFile(t, leaf, `{"y":1}`)

	cs := NewConfigStore(root)
	_, rev1, err := cs.Resolve(ScopeInfra, "", "metric-store", "ms01")
	if err != nil {
		t.Fatal(err)
	}

	// Rewrite the leaf with different content and a bumped mtime, then reload
	// so the new generation is published.
	writeFile(t, leaf, `{"y":2}`)
	future := time.Unix(1<<40, 0)
	if err := os.Chtimes(leaf, future, future); err != nil {
		t.Fatal(err)
	}
	if err := cs.Reload(); err != nil {
		t.Fatal(err)
	}

	_, rev2, err := cs.Resolve(ScopeInfra, "", "metric-store", "ms01")
	if err != nil {
		t.Fatal(err)
	}
	if rev1 == rev2 {
		t.Errorf("revision unchanged after edit: %d", rev1)
	}
}

func TestConfigStoreErrors(t *testing.T) {
	t.Run("no config", func(t *testing.T) {
		cs := NewConfigStore(t.TempDir())
		_, _, err := cs.Resolve(ScopeInfra, "", "metric-store", "nope")
		if !errors.Is(err, ErrNoConfig) {
			t.Fatalf("want ErrNoConfig, got %v", err)
		}
	})

	t.Run("malformed json is not published", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "bad", "h1.json"), `{not json`)
		cs := NewConfigStore(root)
		// A malformed tree must fail the load rather than publish a snapshot.
		if err := cs.Reload(); err == nil || errors.Is(err, ErrNoConfig) {
			t.Fatalf("want parse error from Reload, got %v", err)
		}
		_, _, err := cs.Resolve(ScopeInfra, "", "bad", "h1")
		if err == nil || errors.Is(err, ErrNoConfig) {
			t.Fatalf("want parse error, got %v", err)
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		cs := NewConfigStore(t.TempDir())
		_, _, err := cs.Resolve(ScopeInfra, "", "../etc", "h1")
		if err == nil {
			t.Fatal("want error for traversal in service_type")
		}
	})

	t.Run("unknown scope", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "defaults.json"), `{"a":1}`)
		cs := NewConfigStore(root)
		_, _, err := cs.Resolve("bogus", "", "t", "h")
		if err == nil {
			t.Fatal("want error for unknown scope")
		}
	})
}

func TestConfigStoreLastKnownGood(t *testing.T) {
	root := t.TempDir()
	leaf := filepath.Join(root, "metric-store", "ms01.json")
	writeFile(t, leaf, `{"y":1}`)

	cs := NewConfigStore(root)
	if err := cs.Reload(); err != nil {
		t.Fatal(err)
	}
	blob1, rev1, err := cs.Resolve(ScopeInfra, "", "metric-store", "ms01")
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the file (simulating a mid-edit / broken save) and reload:
	// the reload must fail and NOT publish, so the previous good snapshot
	// keeps serving.
	writeFile(t, leaf, `{broken`)
	future := time.Unix(1<<40, 0)
	if err := os.Chtimes(leaf, future, future); err != nil {
		t.Fatal(err)
	}
	if err := cs.Reload(); err == nil {
		t.Fatal("reload of a broken tree should fail")
	}
	blob2, rev2, err := cs.Resolve(ScopeInfra, "", "metric-store", "ms01")
	if err != nil {
		t.Fatalf("resolve after broken edit should still serve last-good, got %v", err)
	}
	if rev2 != rev1 || string(blob2) != string(blob1) {
		t.Fatalf("served config changed after a broken edit: %s (rev %d) -> %s (rev %d)",
			blob1, rev1, blob2, rev2)
	}

	// Fix the file: the next reload publishes the corrected generation.
	writeFile(t, leaf, `{"y":2}`)
	if err := os.Chtimes(leaf, future.Add(time.Second), future.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := cs.Reload(); err != nil {
		t.Fatal(err)
	}
	_, rev3, err := cs.Resolve(ScopeInfra, "", "metric-store", "ms01")
	if err != nil {
		t.Fatal(err)
	}
	if rev3 == rev1 {
		t.Fatal("revision should change once the fixed config is published")
	}
}

func TestConfigStoreConcurrentResolveReload(t *testing.T) {
	root := t.TempDir()
	leaf := filepath.Join(root, "metric-store", "ms01.json")
	writeFile(t, leaf, `{"y":0}`)

	cs := NewConfigStore(root)
	if err := cs.Reload(); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Readers hammer Resolve; the writer edits + reloads underneath them.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if _, _, err := cs.Resolve(ScopeInfra, "", "metric-store", "ms01"); err != nil && !errors.Is(err, ErrNoConfig) {
						t.Errorf("resolve: %v", err)
						return
					}
				}
			}
		}()
	}

	base := time.Unix(1<<40, 0)
	for i := 1; i <= 50; i++ {
		writeFile(t, leaf, fmt.Sprintf(`{"y":%d}`, i))
		ts := base.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(leaf, ts, ts); err != nil {
			t.Fatal(err)
		}
		if err := cs.Reload(); err != nil && !errors.Is(err, errTreeChanged) {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}

func TestConfigStoreValidator(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "metric-store", "ms01.json"), `{"y":1}`)

	cs := NewConfigStore(root)
	sentinel := errors.New("rejected")
	cs.SetValidator("metric-store", func(json.RawMessage) error { return sentinel })

	_, _, err := cs.Resolve(ScopeInfra, "", "metric-store", "ms01")
	if !errors.Is(err, sentinel) {
		t.Fatalf("want validator error, got %v", err)
	}
}
