// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

import (
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

type Tagger interface {
	Register() error
	Match(job *schema.Job)
}

var (
	initOnce  sync.Once
	jobTagger *JobTagger
)

type JobTagger struct {
	startTaggers []Tagger
	stopTaggers  []Tagger
}

func newTagger() {
	jobTagger = &JobTagger{}
	jobTagger.startTaggers = make([]Tagger, 0)
	jobTagger.startTaggers = append(jobTagger.startTaggers, &AppTagger{})
	jobTagger.stopTaggers = make([]Tagger, 0)
	jobTagger.stopTaggers = append(jobTagger.startTaggers, &JobClassTagger{})

	for _, tagger := range jobTagger.startTaggers {
		tagger.Register()
	}
}

func Init() {
	initOnce.Do(func() {
		newTagger()
		repository.RegisterJobJook(jobTagger)
	})
}

func (jt *JobTagger) JobStartCallback(job *schema.Job) {
	for _, tagger := range jt.startTaggers {
		tagger.Match(job)
	}
}

func (jt *JobTagger) JobStopCallback(job *schema.Job) {
	for _, tagger := range jt.stopTaggers {
		tagger.Match(job)
	}
}

func RunTaggers() error {
	newTagger()
	r := repository.GetJobRepository()
	jl, err := r.GetJobList()
	if err != nil {
		log.Errorf("Error while getting job list %s", err)
		return err
	}

	for _, id := range jl {
		job, err := r.FindByIdDirect(id)
		if err != nil {
			log.Errorf("Error while getting job %s", err)
			return err
		}
		for _, tagger := range jobTagger.startTaggers {
			tagger.Match(job)
		}
		for _, tagger := range jobTagger.stopTaggers {
			tagger.Match(job)
		}
	}
	return nil
}
