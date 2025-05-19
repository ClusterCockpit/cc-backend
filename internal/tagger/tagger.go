// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

import (
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
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

func Init() {
	initOnce.Do(func() {
		jobTagger = &JobTagger{}
		jobTagger.startTaggers = make([]Tagger, 0)
		jobTagger.startTaggers = append(jobTagger.startTaggers, &AppTagger{})

		for _, tagger := range jobTagger.startTaggers {
			tagger.Register()
		}

		// jobTagger.stopTaggers = make([]Tagger, 0)
		repository.RegisterJobJook(jobTagger)
	})
}

func (jt *JobTagger) JobStartCallback(job *schema.Job) {
	for _, tagger := range jobTagger.startTaggers {
		tagger.Match(job)
	}
}

func (jt *JobTagger) JobStopCallback(job *schema.Job) {
}
