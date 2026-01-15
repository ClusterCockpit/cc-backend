// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"sync"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// JobHook interface allows external components to hook into job lifecycle events.
// Implementations can perform actions when jobs start or stop, such as tagging,
// logging, notifications, or triggering external workflows.
//
// Example implementation:
//
//	type MyJobTagger struct{}
//
//	func (t *MyJobTagger) JobStartCallback(job *schema.Job) {
//	    if job.NumNodes > 100 {
//	        // Tag large jobs automatically
//	    }
//	}
//
//	func (t *MyJobTagger) JobStopCallback(job *schema.Job) {
//	    if job.State == schema.JobStateFailed {
//	        // Log or alert on failed jobs
//	    }
//	}
//
// Register hooks during application initialization:
//
//	repository.RegisterJobHook(&MyJobTagger{})
type JobHook interface {
	// JobStartCallback is invoked when one or more jobs start.
	// This is called synchronously, so implementations should be fast.
	JobStartCallback(job *schema.Job)

	// JobStopCallback is invoked when a job completes.
	// This is called synchronously, so implementations should be fast.
	JobStopCallback(job *schema.Job)
}

var (
	initOnce sync.Once
	hooks    []JobHook
)

// RegisterJobHook registers a JobHook to receive job lifecycle callbacks.
// Multiple hooks can be registered and will be called in registration order.
// This function is safe to call multiple times and is typically called during
// application initialization.
//
// Nil hooks are silently ignored to simplify conditional registration.
func RegisterJobHook(hook JobHook) {
	initOnce.Do(func() {
		hooks = make([]JobHook, 0)
	})

	if hook != nil {
		hooks = append(hooks, hook)
	}
}

// CallJobStartHooks invokes all registered JobHook.JobStartCallback methods
// for each job in the provided slice. This is called internally by the repository
// when jobs are started (e.g., via StartJob or batch job imports).
//
// Hooks are called synchronously in registration order. If a hook panics,
// the panic will propagate to the caller.
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

// CallJobStopHooks invokes all registered JobHook.JobStopCallback methods
// for the provided job. This is called internally by the repository when a
// job completes (e.g., via StopJob or job state updates).
//
// Hooks are called synchronously in registration order. If a hook panics,
// the panic will propagate to the caller.
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
