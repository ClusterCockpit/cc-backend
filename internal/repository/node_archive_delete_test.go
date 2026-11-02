// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jmoiron/sqlx"
)

func archiveDeleteTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "archive-delete.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE node(id INTEGER PRIMARY KEY,hostname TEXT,cluster TEXT);
CREATE TABLE node_state(id INTEGER PRIMARY KEY,time_stamp INTEGER,node_id INTEGER);
INSERT INTO node VALUES(1,'node','cluster');`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestDeleteArchivedNodeStatesBoundsStatementsAndProtectsLatestTies(t *testing.T) {
	db := archiveDeleteTestDB(t)
	var archived []NodeStateWithNode
	for id := int64(1); id <= 400; id++ {
		if _, err := db.Exec("INSERT INTO node_state VALUES(?,?,1)", id, id); err != nil {
			t.Fatal(err)
		}
		archived = append(archived, NodeStateWithNode{ID: id, TimeStamp: id, Hostname: "node", Cluster: "cluster"})
	}
	_, err := db.Exec("INSERT INTO node_state VALUES(501,1000,1),(502,1000,1),(600,1,NULL),(700,1,1)")
	if err != nil {
		t.Fatal(err)
	}
	// Even if a previously selected row is now latest, every timestamp tie
	// must survive. NULL/unselected rows must not become collateral cleanup.
	for _, id := range []int64{501, 502} {
		archived = append(archived, NodeStateWithNode{ID: id, TimeStamp: 1000, Hostname: "node", Cluster: "cluster"})
	}
	if err := (&NodeRepository{DB: db}).DeleteNodeStatesByIDs(context.Background(), archived); err != nil {
		t.Fatal(err)
	}
	var got []int64
	if err := db.Select(&got, "SELECT id FROM node_state ORDER BY id"); err != nil {
		t.Fatal(err)
	}
	if want := []int64{501, 502, 600, 700}; !slices.Equal(got, want) {
		t.Fatalf("remaining IDs %v; want %v", got, want)
	}
}

func TestDeleteArchivedNodeStatesHonorsCancellation(t *testing.T) {
	db := archiveDeleteTestDB(t)
	if _, err := db.Exec("INSERT INTO node_state VALUES(1,1,1),(2,2,1)"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (&NodeRepository{DB: db}).DeleteNodeStatesByIDs(ctx, []NodeStateWithNode{{ID: 1, TimeStamp: 1, Hostname: "node", Cluster: "cluster"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled deletion accepted: %v", err)
	}
	var remaining int
	if err := db.QueryRow("SELECT COUNT(*) FROM node_state").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("canceled deletion changed source rows: %d", remaining)
	}
	if err := (&NodeRepository{}).DeleteNodeStatesByIDs(context.Background(), nil); err != nil {
		t.Fatalf("empty archived selection failed: %v", err)
	}
}
