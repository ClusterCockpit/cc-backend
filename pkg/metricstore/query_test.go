// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-line-protocol/v2/lineprotocol"
)

const deviceCluster = "devcluster"

var deviceArchiveOnce sync.Once

// initDeviceArchive initializes the global archive once per test binary with
// testdata/device-cluster.json: subcluster a declares /home and /scratch, b
// declares /work, c declares no filesystems. The cluster configuration is held
// in memory after Init, so the temporary archive is removed right away.
func initDeviceArchive(t *testing.T) {
	t.Helper()
	var err error
	deviceArchiveOnce.Do(func() {
		var dir string
		dir, err = os.MkdirTemp("", "device-archive")
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)

		var clusterJSON []byte
		if clusterJSON, err = os.ReadFile(filepath.Join("testdata", "device-cluster.json")); err != nil {
			return
		}
		if err = os.WriteFile(filepath.Join(dir, "version.txt"), fmt.Appendf(nil, "%d\n", archive.Version), 0o644); err != nil {
			return
		}
		if err = os.Mkdir(filepath.Join(dir, deviceCluster), 0o755); err != nil {
			return
		}
		if err = os.WriteFile(filepath.Join(dir, deviceCluster, "cluster.json"), clusterJSON, 0o644); err != nil {
			return
		}
		err = archive.Init(json.RawMessage(fmt.Sprintf(`{"kind": "file", "path": %q}`, dir)))
	})
	if err != nil {
		t.Fatalf("init device archive: %v", err)
	}
	if archive.GetCluster(deviceCluster) == nil {
		t.Fatal("device archive not initialized")
	}
}

func TestDeviceArchiveFixture(t *testing.T) {
	initDeviceArchive(t)

	want := map[string][]string{"a": {"/home", "/scratch"}, "b": {"/work"}, "c": {}}
	for sc, ids := range want {
		subCluster, err := archive.GetSubCluster(deviceCluster, sc)
		if err != nil {
			t.Fatalf("GetSubCluster(%s): %v", sc, err)
		}
		if got := subCluster.Topology.GetFilesystemIDs(); !slices.Equal(got, ids) {
			t.Errorf("subcluster %s filesystems = %v, want %v", sc, got, ids)
		}
	}
}

func deviceJob(subCluster string, hosts ...string) *schema.Job {
	job := &schema.Job{Cluster: deviceCluster, SubCluster: subCluster}
	for _, h := range hosts {
		job.Resources = append(job.Resources, &schema.Resource{Hostname: h})
	}
	return job
}

func TestBuildQueriesDeviceScopes(t *testing.T) {
	initDeviceArchive(t)

	t.Run("device request does not drop hwthread", func(t *testing.T) {
		queries, targets, err := buildQueries(deviceJob("a", "a01"), []string{"flops_any"},
			[]schema.MetricScope{schema.MetricScopeFilesystem, schema.MetricScopeHWThread}, 60)
		if err != nil {
			t.Fatal(err)
		}
		if len(queries) != 1 || *queries[0].Type != HWThreadString || targets[0].Scope != schema.MetricScopeHWThread {
			t.Fatalf("queries = %+v, targets = %v; want one hwthread query", queries, targets)
		}
		if targets[0].ID != nil {
			t.Errorf("hwthread query target id = %q, want none", *targets[0].ID)
		}
		if !slices.Equal(queries[0].TypeIds, []string{"0", "1", "2", "3"}) {
			t.Errorf("hwthread ids = %v", queries[0].TypeIds)
		}
	})

	t.Run("declared filesystems are queried", func(t *testing.T) {
		queries, targets, err := buildQueries(deviceJob("a", "a01", "a02"), []string{"fs_read_bw"},
			[]schema.MetricScope{schema.MetricScopeFilesystem, schema.MetricScopeNode}, 60)
		if err != nil {
			t.Fatal(err)
		}
		if len(queries) != 4 {
			t.Fatalf("got %d queries, want 4: %+v", len(queries), queries)
		}
		for i, q := range queries {
			if q.Type == nil || *q.Type != FilesystemString || !slices.Equal(q.TypeIds, []string{"/home", "/scratch"}) {
				t.Errorf("query %d = %+v, want type filesystem with ids [/home /scratch]", i, q)
			}
			wantAgg := targets[i].Scope == schema.MetricScopeNode
			if q.Aggregate != wantAgg {
				t.Errorf("query %d at scope %s: aggregate = %v, want %v", i, targets[i].Scope, q.Aggregate, wantAgg)
			}
			if targets[i].ID != nil {
				t.Errorf("query %d at scope %s: target id = %q, want none", i, targets[i].Scope, *targets[i].ID)
			}
		}
	})

	t.Run("core targets carry the core id", func(t *testing.T) {
		queries, targets, err := buildQueries(deviceJob("a", "a01"), []string{"flops_any"},
			[]schema.MetricScope{schema.MetricScopeCore}, 60)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string][]string{"0": {"0", "1"}, "1": {"2", "3"}}
		if len(queries) != len(want) {
			t.Fatalf("got %d queries, want %d: %+v", len(queries), len(want), queries)
		}
		for i, q := range queries {
			target := targets[i]
			if target.Scope != schema.MetricScopeCore || !q.Aggregate || target.ID == nil {
				t.Fatalf("query %d = %+v, target = %+v; want aggregated core query with id", i, q, target)
			}
			if !slices.Equal(q.TypeIds, want[*target.ID]) {
				t.Errorf("core %s: hwthread ids = %v, want %v", *target.ID, q.TypeIds, want[*target.ID])
			}
			delete(want, *target.ID)
		}
		if len(want) != 0 {
			t.Errorf("cores without a query: %v", want)
		}
	})

	t.Run("subcluster without filesystems", func(t *testing.T) {
		queries, _, err := buildQueries(deviceJob("c", "c01"), []string{"fs_read_bw"},
			[]schema.MetricScope{schema.MetricScopeNode, schema.MetricScopeFilesystem}, 60)
		if err != nil {
			t.Fatal(err)
		}
		if len(queries) != 0 {
			t.Errorf("got %d queries, want none: %+v", len(queries), queries)
		}
	})

	t.Run("allocated accelerators only", func(t *testing.T) {
		job := deviceJob("a", "a01")
		job.Resources[0].Accelerators = []string{"1"}
		queries, _, err := buildQueries(job, []string{"acc_util"},
			[]schema.MetricScope{schema.MetricScopeAccelerator}, 60)
		if err != nil {
			t.Fatal(err)
		}
		if len(queries) != 1 || *queries[0].Type != AcceleratorString || !slices.Equal(queries[0].TypeIds, []string{"1"}) {
			t.Errorf("queries = %+v, want one accelerator query for id 1", queries)
		}
	})
}

func TestBuildNodeQueriesPerNodeSubCluster(t *testing.T) {
	initDeviceArchive(t)

	queries, targets, err := buildNodeQueries(deviceCluster, "", []string{"a01", "b01", "c01"},
		[]string{"fs_read_bw"}, []schema.MetricScope{schema.MetricScopeFilesystem}, 60)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string][]string{"a01": {"/home", "/scratch"}, "b01": {"/work"}}
	if len(queries) != len(want) {
		t.Fatalf("got %d queries, want %d: %+v", len(queries), len(want), queries)
	}
	for i, q := range queries {
		if targets[i].Scope != schema.MetricScopeFilesystem || q.Aggregate || targets[i].ID != nil {
			t.Errorf("query %d: target %+v aggregate %v, want unaggregated filesystem without target id", i, targets[i], q.Aggregate)
		}
		if ids, ok := want[q.Hostname]; !ok || !slices.Equal(q.TypeIds, ids) {
			t.Errorf("host %s ids = %v, want %v", q.Hostname, q.TypeIds, want[q.Hostname])
		}
	}
}

// TestLoadDataSeriesIDs runs the full LoadData path for a filesystem metric:
// the node series aggregates both mounts and carries no id, while the
// filesystem series carry their mount points.
func TestLoadDataSeriesIDs(t *testing.T) {
	initDeviceArchive(t)

	ms := newDeviceTestStore()
	now := time.Now().Unix() / 60 * 60
	var b strings.Builder
	for ts := now - 300; ts <= now; ts += 60 {
		fmt.Fprintf(&b, "fs_read_bw,cluster=%s,hostname=a01,type=filesystem,type-id=/home value=1 %d\n", deviceCluster, ts)
		fmt.Fprintf(&b, "fs_read_bw,cluster=%s,hostname=a01,type=filesystem,type-id=/scratch value=2 %d\n", deviceCluster, ts)
	}
	if err := DecodeLine(lineprotocol.NewDecoderWithBytes([]byte(b.String())), ms, deviceCluster); err != nil {
		t.Fatalf("DecodeLine: %v", err)
	}
	old := msInstance
	msInstance = ms
	t.Cleanup(func() { msInstance = old })

	job := deviceJob("a", "a01")
	job.StartTime = now - 240
	job.Duration = 240
	jobData, err := (&InternalMetricStore{}).LoadData(job, []string{"fs_read_bw"},
		[]schema.MetricScope{schema.MetricScopeNode, schema.MetricScopeFilesystem},
		context.Background(), 60, "")
	if err != nil {
		t.Fatal(err)
	}

	scoped := jobData.Metrics["fs_read_bw"]
	node := scoped[schema.MetricScopeNode]
	if node == nil || len(node.Series) != 1 {
		t.Fatalf("node scope = %+v, want one series", node)
	}
	if id := node.Series[0].ID; id != nil {
		t.Errorf("node series id = %q, want none", *id)
	}
	if node.Series[0].Statistics.Avg != 3 {
		t.Errorf("node series avg = %v, want the sum 3", node.Series[0].Statistics.Avg)
	}

	fs := scoped[schema.MetricScopeFilesystem]
	if fs == nil {
		t.Fatal("no filesystem series")
	}
	var ids []string
	for _, s := range fs.Series {
		if s.ID == nil {
			t.Fatalf("filesystem series without id: %+v", s)
		}
		ids = append(ids, *s.ID)
	}
	if !slices.Equal(ids, []string{"/home", "/scratch"}) {
		t.Errorf("filesystem series ids = %v, want [/home /scratch]", ids)
	}
}
