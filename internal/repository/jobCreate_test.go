// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"encoding/json"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestJob creates a minimal valid job for testing
func createTestJob(jobID int64, cluster string) *schema.Job {
	return &schema.Job{
		JobID:            jobID,
		User:             "testuser",
		Project:          "testproject",
		Cluster:          cluster,
		SubCluster:       "main",
		Partition:        "batch",
		NumNodes:         1,
		NumHWThreads:     4,
		NumAcc:           0,
		Shared:           "none",
		MonitoringStatus: schema.MonitoringStatusRunningOrArchiving,
		SMT:              1,
		State:            schema.JobStateRunning,
		StartTime:        1234567890,
		Duration:         0,
		Walltime:         3600,
		Resources: []*schema.Resource{
			{
				Hostname:  "node01",
				HWThreads: []int{0, 1, 2, 3},
			},
		},
		Footprint: map[string]float64{
			"cpu_load":      50.0,
			"mem_used":      8000.0,
			"flops_any":     0.5,
			"mem_bw":        10.0,
			"net_bw":        2.0,
			"file_bw":       1.0,
			"cpu_used":      2.0,
			"cpu_load_core": 12.5,
		},
		MetaData: map[string]string{
			"jobName":     "test_job",
			"queue":       "normal",
			"qosName":     "default",
			"accountName": "testaccount",
		},
	}
}

func TestInsertJob(t *testing.T) {
	r := setup(t)

	t.Run("successful insertion", func(t *testing.T) {
		job := createTestJob(999001, "testcluster")
		job.RawResources, _ = json.Marshal(job.Resources)
		job.RawFootprint, _ = json.Marshal(job.Footprint)
		job.RawMetaData, _ = json.Marshal(job.MetaData)

		id, err := r.InsertJob(job)
		require.NoError(t, err, "InsertJob should succeed")
		assert.Greater(t, id, int64(0), "Should return valid insert ID")

		// Verify job was inserted into job_cache
		var count int
		err = r.DB.QueryRow("SELECT COUNT(*) FROM job_cache WHERE job_id = ? AND cluster = ?",
			job.JobID, job.Cluster).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Job should be in job_cache table")

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job_cache WHERE job_id = ? AND cluster = ?", job.JobID, job.Cluster)
		require.NoError(t, err)
	})

	t.Run("insertion with all fields", func(t *testing.T) {
		job := createTestJob(999002, "testcluster")
		job.ArrayJobID = 5000
		job.Energy = 1500.5
		job.RawResources, _ = json.Marshal(job.Resources)
		job.RawFootprint, _ = json.Marshal(job.Footprint)
		job.RawMetaData, _ = json.Marshal(job.MetaData)

		id, err := r.InsertJob(job)
		require.NoError(t, err)
		assert.Greater(t, id, int64(0))

		// Verify all fields were stored correctly
		var retrievedJob schema.Job
		err = r.DB.QueryRow(`SELECT job_id, hpc_user, project, cluster, array_job_id, energy 
			FROM job_cache WHERE id = ?`, id).Scan(
			&retrievedJob.JobID, &retrievedJob.User, &retrievedJob.Project,
			&retrievedJob.Cluster, &retrievedJob.ArrayJobID, &retrievedJob.Energy)
		require.NoError(t, err)
		assert.Equal(t, job.JobID, retrievedJob.JobID)
		assert.Equal(t, job.User, retrievedJob.User)
		assert.Equal(t, job.Project, retrievedJob.Project)
		assert.Equal(t, job.Cluster, retrievedJob.Cluster)
		assert.Equal(t, job.ArrayJobID, retrievedJob.ArrayJobID)
		assert.Equal(t, job.Energy, retrievedJob.Energy)

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job_cache WHERE id = ?", id)
		require.NoError(t, err)
	})
}

func TestStart(t *testing.T) {
	r := setup(t)

	t.Run("successful job start with JSON encoding", func(t *testing.T) {
		job := createTestJob(999003, "testcluster")

		id, err := r.Start(job)
		require.NoError(t, err, "Start should succeed")
		assert.Greater(t, id, int64(0), "Should return valid insert ID")

		// Verify job was inserted and JSON fields were encoded
		var rawResources, rawFootprint, rawMetaData []byte
		err = r.DB.QueryRow(`SELECT resources, footprint, meta_data FROM job_cache WHERE id = ?`, id).Scan(
			&rawResources, &rawFootprint, &rawMetaData)
		require.NoError(t, err)

		// Verify resources JSON
		var resources []*schema.Resource
		err = json.Unmarshal(rawResources, &resources)
		require.NoError(t, err, "Resources should be valid JSON")
		assert.Equal(t, 1, len(resources))
		assert.Equal(t, "node01", resources[0].Hostname)

		// Verify footprint JSON
		var footprint map[string]float64
		err = json.Unmarshal(rawFootprint, &footprint)
		require.NoError(t, err, "Footprint should be valid JSON")
		assert.Equal(t, 50.0, footprint["cpu_load"])
		assert.Equal(t, 8000.0, footprint["mem_used"])

		// Verify metadata JSON
		var metaData map[string]string
		err = json.Unmarshal(rawMetaData, &metaData)
		require.NoError(t, err, "MetaData should be valid JSON")
		assert.Equal(t, "test_job", metaData["jobName"])

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job_cache WHERE id = ?", id)
		require.NoError(t, err)
	})

	t.Run("job start with empty footprint", func(t *testing.T) {
		job := createTestJob(999004, "testcluster")
		job.Footprint = map[string]float64{}

		id, err := r.Start(job)
		require.NoError(t, err)
		assert.Greater(t, id, int64(0))

		// Verify empty footprint was encoded as empty JSON object
		var rawFootprint []byte
		err = r.DB.QueryRow(`SELECT footprint FROM job_cache WHERE id = ?`, id).Scan(&rawFootprint)
		require.NoError(t, err)
		assert.Equal(t, []byte("{}"), rawFootprint)

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job_cache WHERE id = ?", id)
		require.NoError(t, err)
	})

	t.Run("job start with nil metadata", func(t *testing.T) {
		job := createTestJob(999005, "testcluster")
		job.MetaData = nil

		id, err := r.Start(job)
		require.NoError(t, err)
		assert.Greater(t, id, int64(0))

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job_cache WHERE id = ?", id)
		require.NoError(t, err)
	})
}

func TestStop(t *testing.T) {
	r := setup(t)

	t.Run("successful job stop", func(t *testing.T) {
		// First insert a job using Start
		job := createTestJob(999106, "testcluster")
		id, err := r.Start(job)
		require.NoError(t, err)

		// Move from job_cache to job table (simulate SyncJobs) - exclude id to let it auto-increment
		_, err = r.DB.Exec(`INSERT INTO job (job_id, cluster, subcluster, submit_time, start_time, hpc_user, project, 
			cluster_partition, array_job_id, duration, walltime, job_state, meta_data, resources, num_nodes, 
			num_hwthreads, num_acc, smt, shared, monitoring_status, energy, energy_footprint, footprint) 
			SELECT job_id, cluster, subcluster, submit_time, start_time, hpc_user, project, 
			cluster_partition, array_job_id, duration, walltime, job_state, meta_data, resources, num_nodes, 
			num_hwthreads, num_acc, smt, shared, monitoring_status, energy, energy_footprint, footprint 
			FROM job_cache WHERE id = ?`, id)
		require.NoError(t, err)
		_, err = r.DB.Exec("DELETE FROM job_cache WHERE id = ?", id)
		require.NoError(t, err)

		// Get the new job id in the job table
		err = r.DB.QueryRow("SELECT id FROM job WHERE job_id = ? AND cluster = ? AND start_time = ?",
			job.JobID, job.Cluster, job.StartTime).Scan(&id)
		require.NoError(t, err)

		// Stop the job
		duration := int32(3600)
		state := schema.JobStateCompleted
		monitoringStatus := int32(schema.MonitoringStatusArchivingSuccessful)

		err = r.Stop(id, duration, state, monitoringStatus)
		require.NoError(t, err, "Stop should succeed")

		// Verify job was updated
		var retrievedDuration int32
		var retrievedState string
		var retrievedMonStatus int32
		err = r.DB.QueryRow(`SELECT duration, job_state, monitoring_status FROM job WHERE id = ?`, id).Scan(
			&retrievedDuration, &retrievedState, &retrievedMonStatus)
		require.NoError(t, err)
		assert.Equal(t, duration, retrievedDuration)
		assert.Equal(t, string(state), retrievedState)
		assert.Equal(t, monitoringStatus, retrievedMonStatus)

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job WHERE id = ?", id)
		require.NoError(t, err)
	})

	t.Run("stop updates job state transitions", func(t *testing.T) {
		// Insert a job
		job := createTestJob(999107, "testcluster")
		id, err := r.Start(job)
		require.NoError(t, err)

		// Move to job table
		_, err = r.DB.Exec(`INSERT INTO job (job_id, cluster, subcluster, submit_time, start_time, hpc_user, project, 
			cluster_partition, array_job_id, duration, walltime, job_state, meta_data, resources, num_nodes, 
			num_hwthreads, num_acc, smt, shared, monitoring_status, energy, energy_footprint, footprint) 
			SELECT job_id, cluster, subcluster, submit_time, start_time, hpc_user, project, 
			cluster_partition, array_job_id, duration, walltime, job_state, meta_data, resources, num_nodes, 
			num_hwthreads, num_acc, smt, shared, monitoring_status, energy, energy_footprint, footprint 
			FROM job_cache WHERE id = ?`, id)
		require.NoError(t, err)
		_, err = r.DB.Exec("DELETE FROM job_cache WHERE id = ?", id)
		require.NoError(t, err)

		// Get the new job id in the job table
		err = r.DB.QueryRow("SELECT id FROM job WHERE job_id = ? AND cluster = ? AND start_time = ?",
			job.JobID, job.Cluster, job.StartTime).Scan(&id)
		require.NoError(t, err)

		// Stop the job with different duration
		err = r.Stop(id, 7200, schema.JobStateCompleted, int32(schema.MonitoringStatusArchivingSuccessful))
		require.NoError(t, err)

		// Verify the duration was updated correctly
		var duration int32
		err = r.DB.QueryRow(`SELECT duration FROM job WHERE id = ?`, id).Scan(&duration)
		require.NoError(t, err)
		assert.Equal(t, int32(7200), duration, "Duration should be updated to 7200")

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job WHERE id = ?", id)
		require.NoError(t, err)
	})

	t.Run("stop with different states", func(t *testing.T) {
		testCases := []struct {
			name             string
			jobID            int64
			state            schema.JobState
			monitoringStatus int32
		}{
			{"completed", 999108, schema.JobStateCompleted, int32(schema.MonitoringStatusArchivingSuccessful)},
			{"failed", 999118, schema.JobStateFailed, int32(schema.MonitoringStatusArchivingSuccessful)},
			{"cancelled", 999119, schema.JobStateCancelled, int32(schema.MonitoringStatusArchivingSuccessful)},
			{"timeout", 999120, schema.JobStateTimeout, int32(schema.MonitoringStatusArchivingSuccessful)},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				job := createTestJob(tc.jobID, "testcluster")
				id, err := r.Start(job)
				require.NoError(t, err)

				// Move to job table
				_, err = r.DB.Exec(`INSERT INTO job (job_id, cluster, subcluster, submit_time, start_time, hpc_user, project, 
					cluster_partition, array_job_id, duration, walltime, job_state, meta_data, resources, num_nodes, 
					num_hwthreads, num_acc, smt, shared, monitoring_status, energy, energy_footprint, footprint) 
					SELECT job_id, cluster, subcluster, submit_time, start_time, hpc_user, project, 
					cluster_partition, array_job_id, duration, walltime, job_state, meta_data, resources, num_nodes, 
					num_hwthreads, num_acc, smt, shared, monitoring_status, energy, energy_footprint, footprint 
					FROM job_cache WHERE id = ?`, id)
				require.NoError(t, err)
				_, err = r.DB.Exec("DELETE FROM job_cache WHERE id = ?", id)
				require.NoError(t, err)

				// Get the new job id in the job table
				err = r.DB.QueryRow("SELECT id FROM job WHERE job_id = ? AND cluster = ? AND start_time = ?",
					job.JobID, job.Cluster, job.StartTime).Scan(&id)
				require.NoError(t, err)

				// Stop with specific state
				err = r.Stop(id, 1800, tc.state, tc.monitoringStatus)
				require.NoError(t, err)

				// Verify state was set correctly
				var retrievedState string
				err = r.DB.QueryRow(`SELECT job_state FROM job WHERE id = ?`, id).Scan(&retrievedState)
				require.NoError(t, err)
				assert.Equal(t, string(tc.state), retrievedState)

				// Clean up
				_, err = r.DB.Exec("DELETE FROM job WHERE id = ?", id)
				require.NoError(t, err)
			})
		}
	})
}

func TestStopCached(t *testing.T) {
	r := setup(t)

	t.Run("successful stop cached job", func(t *testing.T) {
		// Insert a job in job_cache
		job := createTestJob(999009, "testcluster")
		id, err := r.Start(job)
		require.NoError(t, err)

		// Stop the cached job
		duration := int32(3600)
		state := schema.JobStateCompleted
		monitoringStatus := int32(schema.MonitoringStatusArchivingSuccessful)

		err = r.StopCached(id, duration, state, monitoringStatus)
		require.NoError(t, err, "StopCached should succeed")

		// Verify job was updated in job_cache table
		var retrievedDuration int32
		var retrievedState string
		var retrievedMonStatus int32
		err = r.DB.QueryRow(`SELECT duration, job_state, monitoring_status FROM job_cache WHERE id = ?`, id).Scan(
			&retrievedDuration, &retrievedState, &retrievedMonStatus)
		require.NoError(t, err)
		assert.Equal(t, duration, retrievedDuration)
		assert.Equal(t, string(state), retrievedState)
		assert.Equal(t, monitoringStatus, retrievedMonStatus)

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job_cache WHERE id = ?", id)
		require.NoError(t, err)
	})

	t.Run("stop cached job does not affect job table", func(t *testing.T) {
		// Insert a job in job_cache
		job := createTestJob(999010, "testcluster")
		id, err := r.Start(job)
		require.NoError(t, err)

		// Stop the cached job
		err = r.StopCached(id, 3600, schema.JobStateCompleted, int32(schema.MonitoringStatusArchivingSuccessful))
		require.NoError(t, err)

		// Verify job table was not affected
		var count int
		err = r.DB.QueryRow(`SELECT COUNT(*) FROM job WHERE job_id = ? AND cluster = ?`,
			job.JobID, job.Cluster).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "Job table should not be affected by StopCached")

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job_cache WHERE id = ?", id)
		require.NoError(t, err)
	})
}

func TestSyncJobs(t *testing.T) {
	r := setup(t)

	t.Run("sync jobs from cache to main table", func(t *testing.T) {
		// Ensure cache is empty first
		_, err := r.DB.Exec("DELETE FROM job_cache")
		require.NoError(t, err)

		// Insert multiple jobs in job_cache
		job1 := createTestJob(999011, "testcluster")
		job2 := createTestJob(999012, "testcluster")
		job3 := createTestJob(999013, "testcluster")

		_, err = r.Start(job1)
		require.NoError(t, err)
		_, err = r.Start(job2)
		require.NoError(t, err)
		_, err = r.Start(job3)
		require.NoError(t, err)

		// Verify jobs are in job_cache
		var cacheCount int
		err = r.DB.QueryRow("SELECT COUNT(*) FROM job_cache WHERE job_id IN (?, ?, ?)",
			job1.JobID, job2.JobID, job3.JobID).Scan(&cacheCount)
		require.NoError(t, err)
		assert.Equal(t, 3, cacheCount, "All jobs should be in job_cache")

		// Sync jobs
		jobs, err := r.SyncJobs()
		require.NoError(t, err, "SyncJobs should succeed")
		assert.Equal(t, 3, len(jobs), "Should return 3 synced jobs")

		// Verify jobs were moved to job table
		var jobCount int
		err = r.DB.QueryRow("SELECT COUNT(*) FROM job WHERE job_id IN (?, ?, ?)",
			job1.JobID, job2.JobID, job3.JobID).Scan(&jobCount)
		require.NoError(t, err)
		assert.Equal(t, 3, jobCount, "All jobs should be in job table")

		// Verify job_cache was cleared
		err = r.DB.QueryRow("SELECT COUNT(*) FROM job_cache WHERE job_id IN (?, ?, ?)",
			job1.JobID, job2.JobID, job3.JobID).Scan(&cacheCount)
		require.NoError(t, err)
		assert.Equal(t, 0, cacheCount, "job_cache should be empty after sync")

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job WHERE job_id IN (?, ?, ?)", job1.JobID, job2.JobID, job3.JobID)
		require.NoError(t, err)
	})

	t.Run("sync preserves job data", func(t *testing.T) {
		// Ensure cache is empty first
		_, err := r.DB.Exec("DELETE FROM job_cache")
		require.NoError(t, err)

		// Insert a job with specific data
		job := createTestJob(999014, "testcluster")
		job.ArrayJobID = 7777
		job.Energy = 2500.75
		job.Duration = 1800

		id, err := r.Start(job)
		require.NoError(t, err)

		// Update some fields to simulate job progress
		result, err := r.DB.Exec(`UPDATE job_cache SET duration = ?, energy = ? WHERE id = ?`,
			3600, 3000.5, id)
		require.NoError(t, err)
		rowsAffected, _ := result.RowsAffected()
		require.Equal(t, int64(1), rowsAffected, "UPDATE should affect exactly 1 row")

		// Verify the update worked
		var checkDuration int32
		var checkEnergy float64
		err = r.DB.QueryRow(`SELECT duration, energy FROM job_cache WHERE id = ?`, id).Scan(&checkDuration, &checkEnergy)
		require.NoError(t, err)
		require.Equal(t, int32(3600), checkDuration, "Duration should be updated to 3600 before sync")
		require.Equal(t, 3000.5, checkEnergy, "Energy should be updated to 3000.5 before sync")

		// Sync jobs
		jobs, err := r.SyncJobs()
		require.NoError(t, err)
		require.Equal(t, 1, len(jobs), "Should return exactly 1 synced job")

		// Verify in database
		var dbJob schema.Job
		err = r.DB.QueryRow(`SELECT job_id, hpc_user, project, cluster, array_job_id, duration, energy 
			FROM job WHERE job_id = ? AND cluster = ?`, job.JobID, job.Cluster).Scan(
			&dbJob.JobID, &dbJob.User, &dbJob.Project, &dbJob.Cluster,
			&dbJob.ArrayJobID, &dbJob.Duration, &dbJob.Energy)
		require.NoError(t, err)
		assert.Equal(t, job.JobID, dbJob.JobID)
		assert.Equal(t, int32(3600), dbJob.Duration)
		assert.Equal(t, 3000.5, dbJob.Energy)

		// Clean up
		_, err = r.DB.Exec("DELETE FROM job WHERE job_id = ? AND cluster = ?", job.JobID, job.Cluster)
		require.NoError(t, err)
	})

	t.Run("sync with empty cache returns empty list", func(t *testing.T) {
		// Ensure cache is empty
		_, err := r.DB.Exec("DELETE FROM job_cache")
		require.NoError(t, err)

		// Sync should return empty list
		jobs, err := r.SyncJobs()
		require.NoError(t, err)
		assert.Equal(t, 0, len(jobs), "Should return empty list when cache is empty")
	})
}
