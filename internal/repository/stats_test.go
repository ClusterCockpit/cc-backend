// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"fmt"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
)

func TestBuildJobStatsQuery(t *testing.T) {
	r := setup(t)
	q := r.buildStatsQuery(nil, "USER")

	sql, _, err := q.ToSql()
	noErr(t, err)

	fmt.Printf("SQL: %s\n", sql)
}

func TestJobStats(t *testing.T) {
	r := setup(t)

	var expectedCount int
	err := r.DB.QueryRow(`SELECT COUNT(*) FROM job`).Scan(&expectedCount)
	noErr(t, err)

	stats, err := r.JobsStats(getContext(t), []*model.JobFilter{})
	noErr(t, err)

	if stats[0].TotalJobs != expectedCount {
		t.Fatalf("Want %d, Got %d", expectedCount, stats[0].TotalJobs)
	}
}
