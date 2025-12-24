// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	_ "github.com/mattn/go-sqlite3"
)

func TestFind(t *testing.T) {
	r := setup(t)

	jobID, cluster, startTime := int64(398800), "fritz", int64(1675954712)
	job, err := r.Find(&jobID, &cluster, &startTime)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Printf("%+v", job)

	if *job.ID != 345 {
		t.Errorf("wrong summary for diagnostic \ngot: %d \nwant: 345", job.JobID)
	}
}

func TestFindById(t *testing.T) {
	r := setup(t)

	job, err := r.FindByID(getContext(t), 338)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Printf("%+v", job)

	if job.JobID != 398793 {
		t.Errorf("wrong summary for diagnostic \ngot: %d \nwant: 1404396", job.JobID)
	}
}

func TestGetTags(t *testing.T) {
	r := setup(t)

	const contextUserKey ContextKey = "user"
	contextUserValue := &schema.User{
		Username:   "testuser",
		Projects:   make([]string, 0),
		Roles:      []string{"user"},
		AuthType:   0,
		AuthSource: 2,
	}

	ctx := context.WithValue(getContext(t), contextUserKey, contextUserValue)

	// Test Tag has Scope "global"
	tags, counts, err := r.CountTags(GetUserFromContext(ctx))
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("TAGS %+v \n", tags)
	// fmt.Printf("COUNTS %+v \n", counts)

	if counts["bandwidth"] != 0 {
		t.Errorf("wrong tag count \ngot: %d \nwant: 0", counts["bandwidth"])
	}
}

func TestFindJobsBetween(t *testing.T) {
	r := setup(t)

	// 1. Find a job to use (Find all jobs)
	// We use a large time range to ensure we get something if it exists
	jobs, err := r.FindJobsBetween(0, 9999999999, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) == 0 {
		t.Fatal("No jobs in test db")
	}

	targetJob := jobs[0]

	// 2. Create a tag
	tagName := fmt.Sprintf("testtag_%d", time.Now().UnixNano())
	tagId, err := r.CreateTag("testtype", tagName, "global")
	if err != nil {
		t.Fatal(err)
	}

	// 3. Link Tag (Manually to avoid archive dependency side-effects in unit test)
	_, err = r.DB.Exec("INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)", *targetJob.ID, tagId)
	if err != nil {
		t.Fatal(err)
	}

	// 4. Search with omitTagged = false (Should find the job)
	jobsFound, err := r.FindJobsBetween(0, 9999999999, false)
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, j := range jobsFound {
		if *j.ID == *targetJob.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Target job %d should be found when omitTagged=false", *targetJob.ID)
	}

	// 5. Search with omitTagged = true (Should NOT find the job)
	jobsFiltered, err := r.FindJobsBetween(0, 9999999999, true)
	if err != nil {
		t.Fatal(err)
	}

	for _, j := range jobsFiltered {
		if *j.ID == *targetJob.ID {
			t.Errorf("Target job %d should NOT be found when omitTagged=true", *targetJob.ID)
		}
	}
}
