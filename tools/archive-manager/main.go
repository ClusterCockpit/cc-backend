// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	ccconf "github.com/ClusterCockpit/cc-lib/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
)

func parseDate(in string) int64 {
	const shortForm = "2006-Jan-02"
	loc, _ := time.LoadLocation("Local")
	if in != "" {
		t, err := time.ParseInLocation(shortForm, in, loc)
		if err != nil {
			cclog.Abortf("Archive Manager Main: Date parse failed with input: '%s'\nError: %s\n", in, err.Error())
		}
		return t.Unix()
	}

	return 0
}

// importArchive imports all jobs from a source archive backend to a destination archive backend.
// It uses parallel processing with a worker pool to improve performance.
// Returns the number of successfully imported jobs, failed jobs, and any error encountered.
func importArchive(srcBackend, dstBackend archive.ArchiveBackend) (int, int, error) {
	cclog.Info("Starting parallel archive import...")

	// Use atomic counters for thread-safe updates
	var imported int32
	var failed int32
	var skipped int32

	// Number of parallel workers
	numWorkers := 4
	cclog.Infof("Using %d parallel workers", numWorkers)

	// Create channels for job distribution
	jobs := make(chan archive.JobContainer, numWorkers*2)
	
	// WaitGroup to track worker completion
	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for job := range jobs {
				// Validate job metadata
				if job.Meta == nil {
					cclog.Warn("Skipping job with nil metadata")
					atomic.AddInt32(&failed, 1)
					continue
				}

				// Validate job data
				if job.Data == nil {
					cclog.Warnf("Job %d from cluster %s has no metric data, skipping",
						job.Meta.JobID, job.Meta.Cluster)
					atomic.AddInt32(&failed, 1)
					continue
				}

				// Check if job already exists in destination
				if dstBackend.Exists(job.Meta) {
					cclog.Debugf("Job %d (cluster: %s, start: %d) already exists in destination, skipping",
						job.Meta.JobID, job.Meta.Cluster, job.Meta.StartTime)
					atomic.AddInt32(&skipped, 1)
					continue
				}

				// Import job to destination
				if err := dstBackend.ImportJob(job.Meta, job.Data); err != nil {
					cclog.Errorf("Failed to import job %d from cluster %s: %s",
						job.Meta.JobID, job.Meta.Cluster, err.Error())
					atomic.AddInt32(&failed, 1)
					continue
				}

				// Successfully imported
				newCount := atomic.AddInt32(&imported, 1)
				if newCount%100 == 0 {
					cclog.Infof("Progress: %d jobs imported, %d skipped, %d failed",
						newCount, atomic.LoadInt32(&skipped), atomic.LoadInt32(&failed))
				}
			}
		}(i)
	}

	// Feed jobs to workers
	go func() {
		for job := range srcBackend.Iter(true) {
			jobs <- job
		}
		close(jobs)
	}()

	// Wait for all workers to complete
	wg.Wait()

	finalImported := int(atomic.LoadInt32(&imported))
	finalFailed := int(atomic.LoadInt32(&failed))
	finalSkipped := int(atomic.LoadInt32(&skipped))

	cclog.Infof("Import completed: %d jobs imported, %d skipped, %d failed",
		finalImported, finalSkipped, finalFailed)

	if finalFailed > 0 {
		return finalImported, finalFailed, fmt.Errorf("%d jobs failed to import", finalFailed)
	}

	return finalImported, finalFailed, nil
}



func main() {
	var srcPath, flagConfigFile, flagLogLevel, flagRemoveCluster, flagRemoveAfter, flagRemoveBefore string
	var flagSrcConfig, flagDstConfig string
	var flagLogDateTime, flagValidate, flagImport bool

	flag.StringVar(&srcPath, "s", "./var/job-archive", "Specify the source job archive path. Default is ./var/job-archive")
	flag.BoolVar(&flagLogDateTime, "logdate", false, "Set this flag to add date and time to log messages")
	flag.StringVar(&flagLogLevel, "loglevel", "warn", "Sets the logging level: `[debug,info,warn (default),err,fatal,crit]`")
	flag.StringVar(&flagConfigFile, "config", "./config.json", "Specify alternative path to `config.json`")
	flag.StringVar(&flagRemoveCluster, "remove-cluster", "", "Remove cluster from archive and database")
	flag.StringVar(&flagRemoveBefore, "remove-before", "", "Remove all jobs with start time before date (Format: 2006-Jan-04)")
	flag.StringVar(&flagRemoveAfter, "remove-after", "", "Remove all jobs with start time after date (Format: 2006-Jan-04)")
	flag.BoolVar(&flagValidate, "validate", false, "Set this flag to validate a job archive against the json schema")
	flag.BoolVar(&flagImport, "import", false, "Import jobs from source archive to destination archive")
	flag.StringVar(&flagSrcConfig, "src-config", "", "Source archive backend configuration (JSON), e.g. '{\"kind\":\"file\",\"path\":\"./archive\"}'")
	flag.StringVar(&flagDstConfig, "dst-config", "", "Destination archive backend configuration (JSON), e.g. '{\"kind\":\"sqlite\",\"dbPath\":\"./archive.db\"}'")
	flag.Parse()


	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", srcPath)

	cclog.Init(flagLogLevel, flagLogDateTime)

	// Handle import mode
	if flagImport {
		if flagSrcConfig == "" || flagDstConfig == "" {
			cclog.Fatal("Both --src-config and --dst-config must be specified for import mode")
		}

		cclog.Info("Import mode: initializing source and destination backends...")

		// Initialize source backend
		srcBackend, err := archive.InitBackend(json.RawMessage(flagSrcConfig))
		if err != nil {
			cclog.Fatalf("Failed to initialize source backend: %s", err.Error())
		}
		cclog.Info("Source backend initialized successfully")

		// Initialize destination backend
		dstBackend, err := archive.InitBackend(json.RawMessage(flagDstConfig))
		if err != nil {
			cclog.Fatalf("Failed to initialize destination backend: %s", err.Error())
		}
		cclog.Info("Destination backend initialized successfully")

		// Perform import
		imported, failed, err := importArchive(srcBackend, dstBackend)
		if err != nil {
			cclog.Errorf("Import completed with errors: %s", err.Error())
			if failed > 0 {
				os.Exit(1)
			}
		}

		cclog.Infof("Import finished successfully: %d jobs imported", imported)
		os.Exit(0)
	}

	ccconf.Init(flagConfigFile)


	// Load and check main configuration
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		if clustercfg := ccconf.GetPackageConfig("clusters"); clustercfg != nil {
			config.Init(cfg, clustercfg)
		} else {
			cclog.Abort("Cluster configuration must be present")
		}
	} else {
		cclog.Abort("Main configuration must be present")
	}

	if err := archive.Init(json.RawMessage(archiveCfg), false); err != nil {
		cclog.Fatal(err)
	}
	ar := archive.GetHandle()

	if flagValidate {
		config.Keys.Validate = true
		for job := range ar.Iter(true) {
			cclog.Printf("Validate %s - %d\n", job.Meta.Cluster, job.Meta.JobID)
		}
		os.Exit(0)
	}

	if flagRemoveBefore != "" || flagRemoveAfter != "" {
		ar.Clean(parseDate(flagRemoveBefore), parseDate(flagRemoveAfter))
		os.Exit(0)
	}

	ar.Info()
}
