// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package archiver provides asynchronous job archiving functionality for ClusterCockpit.
//
// The archiver runs a background worker goroutine that processes job archiving requests
// from a buffered channel. When jobs complete, their metric data is archived from the
// metric store to the configured archive backend (filesystem, S3, etc.).
//
// # Architecture
//
// The archiver uses a producer-consumer pattern:
//   - Producer: TriggerArchiving() sends jobs to archiveChannel
//   - Consumer: archivingWorker() processes jobs from the channel
//   - Coordination: sync.WaitGroup tracks pending archive operations
//
// # Lifecycle
//
//  1. Start(repo, ctx) - Initialize worker with context for cancellation
//  2. TriggerArchiving(job) - Queue job for archiving (called when job stops)
//  3. archivingWorker() - Background goroutine processes jobs
//  4. Shutdown(timeout) - Graceful shutdown with timeout
//
// # Graceful Shutdown
//
// The archiver supports graceful shutdown with configurable timeout:
//   - Closes channel to reject new jobs
//   - Waits for pending jobs to complete (up to timeout)
//   - Cancels context if timeout exceeded
//   - Ensures worker goroutine exits cleanly
//
// # Example Usage
//
//	// Initialize archiver
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	archiver.Start(jobRepository, ctx)
//
//	// Trigger archiving when job completes
//	archiver.TriggerArchiving(job)
//
//	// Graceful shutdown with 10 second timeout
//	if err := archiver.Shutdown(10 * time.Second); err != nil {
//	    log.Printf("Archiver shutdown timeout: %v", err)
//	}
package archiver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	sq "github.com/Masterminds/squirrel"
)

var (
	archivePending sync.WaitGroup
	archiveChannel chan *schema.Job
	jobRepo        *repository.JobRepository
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	workerDone     chan struct{}
)

// Start initializes the archiver and starts the background worker goroutine.
//
// The archiver processes job archiving requests asynchronously via a buffered channel.
// Jobs are sent to the channel using TriggerArchiving() and processed by the worker.
//
// Parameters:
//   - r: JobRepository instance for database operations
//   - ctx: Context for cancellation (shutdown signal propagation)
//
// The worker goroutine will run until:
//   - ctx is cancelled (via parent shutdown)
//   - archiveChannel is closed (via Shutdown())
//
// Must be called before TriggerArchiving(). Safe to call only once.
func Start(r *repository.JobRepository, ctx context.Context) {
	shutdownCtx, shutdownCancel = context.WithCancel(ctx)
	archiveChannel = make(chan *schema.Job, 128)
	workerDone = make(chan struct{})
	jobRepo = r

	go archivingWorker()
}

// archivingWorker is the background goroutine that processes job archiving requests.
//
// The worker loop:
//  1. Blocks waiting for jobs on archiveChannel or shutdown signal
//  2. Fetches job metadata from repository
//  3. Archives job data to configured backend (calls ArchiveJob)
//  4. Updates job footprint and energy metrics in database
//  5. Marks job as successfully archived
//  6. Calls job stop hooks
//
// The worker exits when:
//   - shutdownCtx is cancelled (timeout during shutdown)
//   - archiveChannel is closed (normal shutdown)
//
// Errors during archiving are logged and the job is marked as failed,
// but the worker continues processing other jobs.
func archivingWorker() {
	defer close(workerDone)

	for {
		select {
		case <-shutdownCtx.Done():
			cclog.Info("Archive worker received shutdown signal")
			return

		case job, ok := <-archiveChannel:
			if !ok {
				cclog.Info("Archive channel closed, worker exiting")
				return
			}

			start := time.Now()
			// not using meta data, called to load JobMeta into Cache?
			// will fail if job meta not in repository
			if _, err := jobRepo.FetchMetadata(job); err != nil {
				cclog.Errorf("archiving job (dbid: %d) failed at check metadata step: %s", job.ID, err.Error())
				jobRepo.UpdateMonitoringStatus(*job.ID, schema.MonitoringStatusArchivingFailed)
				archivePending.Done()
				continue
			}

			// ArchiveJob will fetch all the data from a MetricDataRepository and push into configured archive backend
			// Use shutdown context to allow cancellation
			jobMeta, err := ArchiveJob(job, shutdownCtx)
			if err != nil {
				cclog.Errorf("archiving job (dbid: %d) failed at archiving job step: %s", job.ID, err.Error())
				jobRepo.UpdateMonitoringStatus(*job.ID, schema.MonitoringStatusArchivingFailed)
				archivePending.Done()
				continue
			}

			stmt := sq.Update("job").Where("job.id = ?", job.ID)

			if stmt, err = jobRepo.UpdateFootprint(stmt, jobMeta); err != nil {
				cclog.Errorf("archiving job (dbid: %d) failed at update Footprint step: %s", job.ID, err.Error())
				archivePending.Done()
				continue
			}
			if stmt, err = jobRepo.UpdateEnergy(stmt, jobMeta); err != nil {
				cclog.Errorf("archiving job (dbid: %d) failed at update Energy step: %s", job.ID, err.Error())
				archivePending.Done()
				continue
			}
			// Update the jobs database entry one last time:
			stmt = jobRepo.MarkArchived(stmt, schema.MonitoringStatusArchivingSuccessful)
			if err := jobRepo.Execute(stmt); err != nil {
				cclog.Errorf("archiving job (dbid: %d) failed at db execute: %s", job.ID, err.Error())
				archivePending.Done()
				continue
			}
			cclog.Debugf("archiving job %d took %s", job.JobID, time.Since(start))
			cclog.Printf("archiving job (dbid: %d) successful", job.ID)

			repository.CallJobStopHooks(job)
			archivePending.Done()
		}
	}
}

// TriggerArchiving queues a job for asynchronous archiving.
//
// This function should be called when a job completes (stops) to archive its
// metric data from the metric store to the configured archive backend.
//
// The function:
//  1. Increments the pending job counter (WaitGroup)
//  2. Sends the job to the archiving channel (buffered, capacity 128)
//  3. Returns immediately (non-blocking unless channel is full)
//
// The actual archiving is performed asynchronously by the worker goroutine.
// Upon completion, the worker will decrement the pending counter.
//
// Panics if Start() has not been called first.
func TriggerArchiving(job *schema.Job) {
	if archiveChannel == nil {
		cclog.Fatal("Cannot archive without archiving channel. Did you Start the archiver?")
	}

	archivePending.Add(1)
	archiveChannel <- job
}

// Shutdown performs a graceful shutdown of the archiver with a configurable timeout.
//
// The shutdown process:
//  1. Closes archiveChannel - no new jobs will be accepted
//  2. Waits for pending jobs to complete (up to timeout duration)
//  3. If timeout is exceeded:
//     - Cancels shutdownCtx to interrupt ongoing ArchiveJob operations
//     - Returns error indicating timeout
//  4. Waits for worker goroutine to exit cleanly
//
// Parameters:
//   - timeout: Maximum duration to wait for pending jobs to complete
//     (recommended: 10-30 seconds for production)
//
// Returns:
//   - nil if all jobs completed within timeout
//   - error if timeout was exceeded (some jobs may not have been archived)
//
// Jobs that don't complete within the timeout will be marked as failed.
// The function always ensures the worker goroutine exits before returning.
//
// Example:
//
//	if err := archiver.Shutdown(10 * time.Second); err != nil {
//	    log.Printf("Some jobs did not complete: %v", err)
//	}
func Shutdown(timeout time.Duration) error {
	cclog.Info("Initiating archiver shutdown...")

	// Close channel to signal no more jobs will be accepted
	close(archiveChannel)

	// Create a channel to signal when all jobs are done
	done := make(chan struct{})
	go func() {
		archivePending.Wait()
		close(done)
	}()

	// Wait for jobs to complete or timeout
	select {
	case <-done:
		cclog.Info("All archive jobs completed successfully")
		// Wait for worker to exit
		<-workerDone
		return nil
	case <-time.After(timeout):
		cclog.Warn("Archiver shutdown timeout exceeded, cancelling remaining operations")
		// Cancel any ongoing operations
		shutdownCancel()
		// Wait for worker to exit
		<-workerDone
		return fmt.Errorf("archiver shutdown timeout after %v", timeout)
	}
}
