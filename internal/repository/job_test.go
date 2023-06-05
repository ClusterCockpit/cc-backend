// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"fmt"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestFind(t *testing.T) {
	r := setup(t)

	jobId, cluster, startTime := int64(398998), "fritz", int64(1675957496)
	job, err := r.Find(&jobId, &cluster, &startTime)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Printf("%+v", job)

	if job.ID != 5 {
		t.Errorf("wrong summary for diagnostic 3\ngot: %d \nwant: 1366", job.JobID)
	}
}

func TestFindById(t *testing.T) {
	r := setup(t)

	job, err := r.FindById(5)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Printf("%+v", job)

	if job.JobID != 398998 {
		t.Errorf("wrong summary for diagnostic 3\ngot: %d \nwant: 1404396", job.JobID)
	}
}

func TestGetTags(t *testing.T) {
	r := setup(t)

	tags, counts, err := r.CountTags(nil)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("TAGS %+v \n", tags)
	// fmt.Printf("COUNTS %+v \n", counts)

	if counts["bandwidth"] != 0 {
		t.Errorf("wrong tag count \ngot: %d \nwant: 0", counts["bandwidth"])
	}
}
