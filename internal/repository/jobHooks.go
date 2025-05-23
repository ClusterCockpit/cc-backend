// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"sync"

	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

type JobHook interface {
	JobStartCallback(job *schema.Job)
	JobStopCallback(job *schema.Job)
}

var (
	initOnce sync.Once
	hooks    []JobHook
)

func RegisterJobJook(hook JobHook) {
	initOnce.Do(func() {
		hooks = make([]JobHook, 0)
	})

	if hook != nil {
		hooks = append(hooks, hook)
	}
}

func CallJobStartHooks(jobs []*schema.Job) {
	if hooks == nil {
		return
	}

	for _, hook := range hooks {
		if hook != nil {
			for _, job := range jobs {
				hook.JobStartCallback(job)
			}
		}
	}
}

func CallJobStopHooks(job *schema.Job) {
	if hooks == nil {
		return
	}

	for _, hook := range hooks {
		if hook != nil {
			hook.JobStopCallback(job)
		}
	}
}
