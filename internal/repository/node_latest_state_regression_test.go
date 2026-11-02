// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"path/filepath"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/jmoiron/sqlx"
)

func TestNodeLatestStateTimestampTieUsesLatestInsert(t *testing.T) {
	db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "latest-state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE node(id INTEGER PRIMARY KEY,hostname TEXT,cluster TEXT,subcluster TEXT);
CREATE TABLE node_state(id INTEGER PRIMARY KEY,time_stamp INTEGER,node_state TEXT,health_state TEXT,node_id INTEGER);
INSERT INTO node VALUES(1,'node','cluster','subcluster');
INSERT INTO node_state VALUES(1,100,'idle','full',1),(2,100,'allocated','failed',1);`)
	if err != nil {
		t.Fatal(err)
	}
	repo := &NodeRepository{DB: db}
	check := func(state schema.SchedulerState, health schema.MonitoringState) {
		t.Helper()
		node, err := repo.GetNode("node", "cluster", false)
		if err != nil {
			t.Fatal(err)
		}
		byID, err := repo.GetNodeByID(1, false)
		if err != nil {
			t.Fatal(err)
		}
		nodes, err := repo.ListNodes("cluster")
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 1 {
			t.Fatalf("node list count %d, want1", len(nodes))
		}
		for _, got := range []*schema.Node{node, byID, nodes[0]} {
			if got.NodeState != state || got.HealthState != health {
				t.Fatalf("latest node state %s/%s, want %s/%s", got.NodeState, got.HealthState, state, health)
			}
		}
		states, healthStates, err := repo.MapNodes("cluster")
		if err != nil {
			t.Fatal(err)
		}
		if states["node"] != string(state) || healthStates["node"] != string(health) {
			t.Fatalf("latest mapped state %v/%v, want %s/%s", states, healthStates, state, health)
		}
	}
	check("allocated", "failed")
	// A higher ID is only the tie-breaker: a genuinely newer timestamp wins
	// even when an older, delayed update is inserted after it.
	_, err = db.Exec("INSERT INTO node_state VALUES(3,101,'mixed','partial',1),(4,100,'idle','full',1)")
	if err != nil {
		t.Fatal(err)
	}
	check("mixed", "partial")
}
