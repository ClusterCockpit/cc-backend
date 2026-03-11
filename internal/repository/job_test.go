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
	jobs, err := r.FindJobsBetween(0, 9999999999, "none")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) == 0 {
		t.Fatal("No jobs in test db")
	}

	targetJob := jobs[0]

	// 2. Create an auto-tagger tag (type "app")
	appTagName := fmt.Sprintf("apptag_%d", time.Now().UnixNano())
	appTagID, err := r.CreateTag("app", appTagName, "global")
	if err != nil {
		t.Fatal(err)
	}

	// 3. Link auto-tagger tag to job
	_, err = r.DB.Exec("INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)", *targetJob.ID, appTagID)
	if err != nil {
		t.Fatal(err)
	}

	// 4. Search with omitTagged = "none" (Should find the job)
	jobsFound, err := r.FindJobsBetween(0, 9999999999, "none")
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
		t.Errorf("Target job %d should be found when omitTagged=none", *targetJob.ID)
	}

	// 5. Search with omitTagged = "all" (Should NOT find the job — it has a tag)
	jobsFiltered, err := r.FindJobsBetween(0, 9999999999, "all")
	if err != nil {
		t.Fatal(err)
	}

	for _, j := range jobsFiltered {
		if *j.ID == *targetJob.ID {
			t.Errorf("Target job %d should NOT be found when omitTagged=all", *targetJob.ID)
		}
	}

	// 6. Search with omitTagged = "user": auto-tagger tag ("app") should NOT exclude the job
	jobsUserFilter, err := r.FindJobsBetween(0, 9999999999, "user")
	if err != nil {
		t.Fatal(err)
	}

	found = false
	for _, j := range jobsUserFilter {
		if *j.ID == *targetJob.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Target job %d should be found when omitTagged=user (only has auto-tagger tag)", *targetJob.ID)
	}

	// 7. Add a user-created tag (type "testtype") to the same job
	userTagName := fmt.Sprintf("usertag_%d", time.Now().UnixNano())
	userTagID, err := r.CreateTag("testtype", userTagName, "global")
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.DB.Exec("INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)", *targetJob.ID, userTagID)
	if err != nil {
		t.Fatal(err)
	}

	// 8. Now omitTagged = "user" should exclude the job (has a user-created tag)
	jobsUserFilter2, err := r.FindJobsBetween(0, 9999999999, "user")
	if err != nil {
		t.Fatal(err)
	}

	for _, j := range jobsUserFilter2 {
		if *j.ID == *targetJob.ID {
			t.Errorf("Target job %d should NOT be found when omitTagged=user (has user-created tag)", *targetJob.ID)
		}
	}
}
