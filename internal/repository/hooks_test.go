// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockJobHook struct {
	startCalled bool
	stopCalled  bool
	startJobs   []*schema.Job
	stopJobs    []*schema.Job
}

func (m *MockJobHook) JobStartCallback(job *schema.Job) {
	m.startCalled = true
	m.startJobs = append(m.startJobs, job)
}

func (m *MockJobHook) JobStopCallback(job *schema.Job) {
	m.stopCalled = true
	m.stopJobs = append(m.stopJobs, job)
}

func TestRegisterJobHook(t *testing.T) {
	t.Run("register single hook", func(t *testing.T) {
		hooks = nil
		mock := &MockJobHook{}

		RegisterJobHook(mock)

		assert.NotNil(t, hooks)
		assert.Len(t, hooks, 1)
		assert.Equal(t, mock, hooks[0])

		hooks = nil
	})

	t.Run("register multiple hooks", func(t *testing.T) {
		hooks = nil
		mock1 := &MockJobHook{}
		mock2 := &MockJobHook{}

		RegisterJobHook(mock1)
		RegisterJobHook(mock2)

		assert.Len(t, hooks, 2)
		assert.Equal(t, mock1, hooks[0])
		assert.Equal(t, mock2, hooks[1])

		hooks = nil
	})

	t.Run("register nil hook does not add to hooks", func(t *testing.T) {
		hooks = nil
		RegisterJobHook(nil)

		if hooks != nil {
			assert.Len(t, hooks, 0, "Nil hook should not be added")
		}

		hooks = nil
	})
}

func TestCallJobStartHooks(t *testing.T) {
	t.Run("call start hooks with single job", func(t *testing.T) {
		hooks = nil
		mock := &MockJobHook{}
		RegisterJobHook(mock)

		job := &schema.Job{
			JobID:   123,
			User:    "testuser",
			Cluster: "testcluster",
		}

		CallJobStartHooks([]*schema.Job{job})

		assert.True(t, mock.startCalled)
		assert.False(t, mock.stopCalled)
		assert.Len(t, mock.startJobs, 1)
		assert.Equal(t, int64(123), mock.startJobs[0].JobID)

		hooks = nil
	})

	t.Run("call start hooks with multiple jobs", func(t *testing.T) {
		hooks = nil
		mock := &MockJobHook{}
		RegisterJobHook(mock)

		jobs := []*schema.Job{
			{JobID: 1, User: "user1", Cluster: "cluster1"},
			{JobID: 2, User: "user2", Cluster: "cluster2"},
			{JobID: 3, User: "user3", Cluster: "cluster3"},
		}

		CallJobStartHooks(jobs)

		assert.True(t, mock.startCalled)
		assert.Len(t, mock.startJobs, 3)
		assert.Equal(t, int64(1), mock.startJobs[0].JobID)
		assert.Equal(t, int64(2), mock.startJobs[1].JobID)
		assert.Equal(t, int64(3), mock.startJobs[2].JobID)

		hooks = nil
	})

	t.Run("call start hooks with multiple registered hooks", func(t *testing.T) {
		hooks = nil
		mock1 := &MockJobHook{}
		mock2 := &MockJobHook{}
		RegisterJobHook(mock1)
		RegisterJobHook(mock2)

		job := &schema.Job{
			JobID: 456, User: "testuser", Cluster: "testcluster",
		}

		CallJobStartHooks([]*schema.Job{job})

		assert.True(t, mock1.startCalled)
		assert.True(t, mock2.startCalled)
		assert.Len(t, mock1.startJobs, 1)
		assert.Len(t, mock2.startJobs, 1)

		hooks = nil
	})

	t.Run("call start hooks with nil hooks", func(t *testing.T) {
		hooks = nil

		job := &schema.Job{
			JobID: 789, User: "testuser", Cluster: "testcluster",
		}

		CallJobStartHooks([]*schema.Job{job})

		hooks = nil
	})

	t.Run("call start hooks with empty job list", func(t *testing.T) {
		hooks = nil
		mock := &MockJobHook{}
		RegisterJobHook(mock)

		CallJobStartHooks([]*schema.Job{})

		assert.False(t, mock.startCalled)
		assert.Len(t, mock.startJobs, 0)

		hooks = nil
	})
}

func TestCallJobStopHooks(t *testing.T) {
	t.Run("call stop hooks with single job", func(t *testing.T) {
		hooks = nil
		mock := &MockJobHook{}
		RegisterJobHook(mock)

		job := &schema.Job{
			JobID:   123,
			User:    "testuser",
			Cluster: "testcluster",
		}

		CallJobStopHooks(job)

		assert.True(t, mock.stopCalled)
		assert.False(t, mock.startCalled)
		assert.Len(t, mock.stopJobs, 1)
		assert.Equal(t, int64(123), mock.stopJobs[0].JobID)

		hooks = nil
	})

	t.Run("call stop hooks with multiple registered hooks", func(t *testing.T) {
		hooks = nil
		mock1 := &MockJobHook{}
		mock2 := &MockJobHook{}
		RegisterJobHook(mock1)
		RegisterJobHook(mock2)

		job := &schema.Job{
			JobID: 456, User: "testuser", Cluster: "testcluster",
		}

		CallJobStopHooks(job)

		assert.True(t, mock1.stopCalled)
		assert.True(t, mock2.stopCalled)
		assert.Len(t, mock1.stopJobs, 1)
		assert.Len(t, mock2.stopJobs, 1)

		hooks = nil
	})

	t.Run("call stop hooks with nil hooks", func(t *testing.T) {
		hooks = nil

		job := &schema.Job{
			JobID: 789, User: "testuser", Cluster: "testcluster",
		}

		CallJobStopHooks(job)

		hooks = nil
	})
}

func TestSQLHooks(t *testing.T) {
	_ = setup(t)

	t.Run("hooks log queries in debug mode", func(t *testing.T) {
		h := &Hooks{}

		ctx := context.Background()
		query := "SELECT * FROM job WHERE job_id = ?"
		args := []any{123}

		ctxWithTime, err := h.Before(ctx, query, args...)
		require.NoError(t, err)
		assert.NotNil(t, ctxWithTime)

		beginTime := ctxWithTime.Value("begin")
		require.NotNil(t, beginTime)
		_, ok := beginTime.(time.Time)
		assert.True(t, ok, "Begin time should be time.Time")

		time.Sleep(10 * time.Millisecond)

		ctxAfter, err := h.After(ctxWithTime, query, args...)
		require.NoError(t, err)
		assert.NotNil(t, ctxAfter)
	})
}

func TestHookIntegration(t *testing.T) {
	t.Run("hooks are called during job lifecycle", func(t *testing.T) {
		hooks = nil
		mock := &MockJobHook{}
		RegisterJobHook(mock)

		job := &schema.Job{
			JobID:   999,
			User:    "integrationuser",
			Cluster: "integrationcluster",
		}

		CallJobStartHooks([]*schema.Job{job})
		assert.True(t, mock.startCalled)
		assert.Equal(t, 1, len(mock.startJobs))

		CallJobStopHooks(job)
		assert.True(t, mock.stopCalled)
		assert.Equal(t, 1, len(mock.stopJobs))

		assert.Equal(t, mock.startJobs[0].JobID, mock.stopJobs[0].JobID)

		hooks = nil
	})
}
