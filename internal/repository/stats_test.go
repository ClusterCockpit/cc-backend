// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
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

	filter := &model.JobFilter{}
	var err error
	var stats []*model.JobsStatistics
	stats, err = r.testJobsStats([]*model.JobFilter{filter})
	noErr(t, err)

	if stats[0].TotalJobs != 98 {
		t.Fatalf("Want 98, Got %d", stats[0].TotalJobs)
	}
}
