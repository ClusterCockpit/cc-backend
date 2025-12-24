// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package tagger provides automatic job tagging functionality for cc-backend.
// It supports detecting applications and classifying jobs based on configurable rules.
// Tags are automatically applied when jobs start or stop, or can be applied retroactively
// to existing jobs using RunTaggers.
package tagger

import (
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// Tagger is the interface that must be implemented by all tagging components.
// Taggers can be registered at job start or stop events to automatically apply tags.
type Tagger interface {
	// Register initializes the tagger and loads any required configuration.
	// It should be called once before the tagger is used.
	Register() error

	// Match evaluates the tagger's rules against a job and applies appropriate tags.
	// It is called for each job that needs to be evaluated.
	Match(job *schema.Job)
}

var (
	initOnce  sync.Once
	jobTagger *JobTagger
)

// JobTagger coordinates multiple taggers that run at different job lifecycle events.
// It maintains separate lists of taggers that run when jobs start and when they stop.
type JobTagger struct {
	// startTaggers are applied when a job starts (e.g., application detection)
	startTaggers []Tagger
	// stopTaggers are applied when a job completes (e.g., job classification)
	stopTaggers []Tagger
}

func newTagger() {
	jobTagger = &JobTagger{}
	jobTagger.startTaggers = make([]Tagger, 0)
	jobTagger.startTaggers = append(jobTagger.startTaggers, &AppTagger{})
	jobTagger.stopTaggers = make([]Tagger, 0)
	jobTagger.stopTaggers = append(jobTagger.stopTaggers, &JobClassTagger{})

	for _, tagger := range jobTagger.startTaggers {
		tagger.Register()
	}
	for _, tagger := range jobTagger.stopTaggers {
		tagger.Register()
	}
}

// Init initializes the job tagger system and registers it with the job repository.
// This function is safe to call multiple times; initialization only occurs once.
// It should be called during application startup.
func Init() {
	initOnce.Do(func() {
		newTagger()
		repository.RegisterJobJook(jobTagger)
	})
}

// JobStartCallback is called when a job starts.
// It runs all registered start taggers (e.g., application detection) on the job.
func (jt *JobTagger) JobStartCallback(job *schema.Job) {
	for _, tagger := range jt.startTaggers {
		tagger.Match(job)
	}
}

// JobStopCallback is called when a job completes.
// It runs all registered stop taggers (e.g., job classification) on the job.
func (jt *JobTagger) JobStopCallback(job *schema.Job) {
	for _, tagger := range jt.stopTaggers {
		tagger.Match(job)
	}
}

// RunTaggers applies all configured taggers to all existing jobs in the repository.
// This is useful for retroactively applying tags to jobs that were created before
// the tagger system was initialized or when new tagging rules are added.
// It fetches all jobs and runs both start and stop taggers on each one.
func RunTaggers() error {
	newTagger()
	r := repository.GetJobRepository()
	jl, err := r.GetJobList(0, 0) // 0 limit means get all jobs (no pagination)
	if err != nil {
		cclog.Errorf("Error while getting job list %s", err)
		return err
	}

	for _, id := range jl {
		job, err := r.FindByIDDirect(id)
		if err != nil {
			cclog.Errorf("Error while getting job %s", err)
			return err
		}
		for _, tagger := range jobTagger.startTaggers {
			tagger.Match(job)
		}
		for _, tagger := range jobTagger.stopTaggers {
			cclog.Infof("Run stop tagger for job %d", job.ID)
			tagger.Match(job)
		}
	}
	return nil
}
