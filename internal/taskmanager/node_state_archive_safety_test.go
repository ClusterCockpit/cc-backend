// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	pqa "github.com/ClusterCockpit/cc-backend/pkg/archive/parquet"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	pq "github.com/parquet-go/parquet-go"
)

var nodeArchiveFailure = errors.New("synthetic node archive failure")

type nodeArchiveSafetyTarget struct {
	files          [][]byte
	writes, failAt int
	beforeReturn   func() error
}

func (target *nodeArchiveSafetyTarget) WriteFile(_ string, data []byte) error {
	target.writes++
	if target.writes == target.failAt {
		return nodeArchiveFailure
	}
	target.files = append(target.files, bytes.Clone(data))
	if target.beforeReturn != nil {
		return target.beforeReturn()
	}
	return nil
}

func nodeArchiveSafetyDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "states.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE node(id INTEGER PRIMARY KEY,hostname TEXT,cluster TEXT,subcluster TEXT);
CREATE TABLE node_state(id INTEGER PRIMARY KEY,time_stamp INTEGER,node_id INTEGER,node_state TEXT,
health_state TEXT,health_metrics TEXT,cpus_allocated INTEGER,memory_allocated INTEGER,gpus_allocated INTEGER,jobs_running INTEGER);
INSERT INTO node VALUES(1,'original','cluster','subcluster'),(2,'other','cluster','subcluster');
INSERT INTO node_state VALUES
(1,1,1,'idle','full',NULL,0,0,0,0),
(2,2,1,'allocated','full',NULL,1,2,0,1),
(3,3,1,'idle','full',NULL,0,0,0,0),
(4,1,NULL,'idle','full',NULL,0,0,0,0),
(10,9,2,'idle','full',NULL,0,0,0,0);`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func requireNodeArchiveIDs(t *testing.T, db *sqlx.DB, want []int64) {
	t.Helper()
	var got []int64
	if err := db.Select(&got, "SELECT id FROM node_state ORDER BY id"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("remaining IDs %v; want %v", got, want)
	}
}

func TestNodeArchiveDeletesOnlyCompletedSelection(t *testing.T) {
	for _, tc := range []struct {
		name, change string
		want         []int64
	}{
		{"late backfill", "INSERT INTO node_state VALUES(100,1,1,'late','full',NULL,0,0,0,0)", []int64{3, 4, 10, 100}},
		{"reused identity", "DELETE FROM node_state WHERE id=1; INSERT INTO node_state VALUES(1,1,2,'replacement','full',NULL,0,0,0,0)", []int64{1, 3, 4, 10}},
		{"latest changed", "DELETE FROM node_state WHERE id=3; INSERT INTO node_state VALUES(100,2,1,'latest-tie','full',NULL,0,0,0,0)", []int64{2, 4, 10, 100}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := nodeArchiveSafetyDB(t)
			ready, release := make(chan struct{}), make(chan struct{})
			target := &nodeArchiveSafetyTarget{beforeReturn: func() error { close(ready); <-release; return nil }}
			done := make(chan error, 1)
			go func() { done <- archiveNodeStatesBefore(&repository.NodeRepository{DB: db}, 10, target, 1) }()
			select {
			case <-ready:
			case err := <-done:
				t.Fatalf("export did not start: %v", err)
			case <-time.After(2 * time.Second):
				t.Fatal("export held a database cursor while writing its target")
			}
			_, mutationErr := db.Exec(tc.change)
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if mutationErr != nil {
				t.Fatal(mutationErr)
			}
			requireNodeArchiveIDs(t, db, tc.want)
			var exported []pqa.ParquetNodeStateRow
			for _, data := range target.files {
				rows, err := pq.Read[pqa.ParquetNodeStateRow](bytes.NewReader(data), int64(len(data)))
				if err != nil {
					t.Fatal(err)
				}
				exported = append(exported, rows...)
			}
			if len(exported) != 2 || exported[0].TimeStamp != 1 || exported[1].TimeStamp != 2 || exported[0].Hostname != "original" || exported[1].Hostname != "original" {
				t.Fatalf("archive does not match original completed selection: %+v", exported)
			}
		})
	}
}

func TestNodeArchiveFailurePreservesAllSourceRows(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			db := nodeArchiveSafetyDB(t)
			target := &nodeArchiveSafetyTarget{failAt: failAt}
			// A zero size forces two separate file writes; the second can fail
			// after the first file has already reached the target.
			err := archiveNodeStatesBefore(&repository.NodeRepository{DB: db}, 10, target, 0)
			if !errors.Is(err, nodeArchiveFailure) {
				t.Fatalf("export failure accepted: %v", err)
			}
			requireNodeArchiveIDs(t, db, []int64{1, 2, 3, 4, 10})
		})
	}
}

func TestNodeArchiveDeleteFailurePreservesSourceRows(t *testing.T) {
	db := nodeArchiveSafetyDB(t)
	_, err := db.Exec(`CREATE TRIGGER reject_archive_delete BEFORE DELETE ON node_state BEGIN SELECT RAISE(ABORT,'synthetic source delete failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	target := &nodeArchiveSafetyTarget{}
	if err := archiveNodeStatesBefore(&repository.NodeRepository{DB: db}, 10, target, 1); err == nil {
		t.Fatal("source deletion failure was accepted")
	}
	if len(target.files) != 1 {
		t.Fatalf("failure did not follow a completed export: %d files", len(target.files))
	}
	requireNodeArchiveIDs(t, db, []int64{1, 2, 3, 4, 10})
}
