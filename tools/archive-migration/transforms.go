// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
)

// transformExclusiveToShared converts the old 'exclusive' field to the new 'shared' field
// Mapping: 0 -> "multi_user", 1 -> "none", 2 -> "single_user"
func transformExclusiveToShared(jobData map[string]interface{}) error {
	// Check if 'exclusive' field exists
	if exclusive, ok := jobData["exclusive"]; ok {
		var exclusiveVal int
		
		// Handle both int and float64 (JSON unmarshaling can produce float64)
		switch v := exclusive.(type) {
		case float64:
			exclusiveVal = int(v)
		case int:
			exclusiveVal = v
		default:
			return fmt.Errorf("exclusive field has unexpected type: %T", exclusive)
		}

		// Map exclusive to shared
		var shared string
		switch exclusiveVal {
		case 0:
			shared = "multi_user"
		case 1:
			shared = "none"
		case 2:
			shared = "single_user"
		default:
			return fmt.Errorf("invalid exclusive value: %d", exclusiveVal)
		}

		// Add shared field and remove exclusive
		jobData["shared"] = shared
		delete(jobData, "exclusive")
		
		cclog.Debugf("Transformed exclusive=%d to shared=%s", exclusiveVal, shared)
	}

	return nil
}

// addMissingFields adds fields that are required in the current schema but might be missing in old archives
func addMissingFields(jobData map[string]interface{}) error {
	// Add submitTime if missing (default to startTime)
	if _, ok := jobData["submitTime"]; !ok {
		if startTime, ok := jobData["startTime"]; ok {
			jobData["submitTime"] = startTime
			cclog.Debug("Added submitTime (defaulted to startTime)")
		}
	}

	// Add energy if missing (default to 0.0)
	if _, ok := jobData["energy"]; !ok {
		jobData["energy"] = 0.0
	}

	// Add requestedMemory if missing (default to 0)
	if _, ok := jobData["requestedMemory"]; !ok {
		jobData["requestedMemory"] = 0
	}

	// Ensure shared field exists (if still missing, default to "none")
	if _, ok := jobData["shared"]; !ok {
		jobData["shared"] = "none"
		cclog.Debug("Added default shared field: none")
	}

	return nil
}

// removeDeprecatedFields removes fields that are no longer in the current schema
func removeDeprecatedFields(jobData map[string]interface{}) error {
	// List of deprecated fields to remove
	deprecatedFields := []string{
		"mem_used_max",
		"flops_any_avg",
		"mem_bw_avg",
		"load_avg",
		"net_bw_avg",
		"net_data_vol_total",
		"file_bw_avg",
		"file_data_vol_total",
	}

	for _, field := range deprecatedFields {
		if _, ok := jobData[field]; ok {
			delete(jobData, field)
			cclog.Debugf("Removed deprecated field: %s", field)
		}
	}

	return nil
}

// migrateJobMetadata applies all transformations to a job metadata map
func migrateJobMetadata(jobData map[string]interface{}) error {
	// Apply transformations in order
	if err := transformExclusiveToShared(jobData); err != nil {
		return fmt.Errorf("transformExclusiveToShared failed: %w", err)
	}

	if err := addMissingFields(jobData); err != nil {
		return fmt.Errorf("addMissingFields failed: %w", err)
	}

	if err := removeDeprecatedFields(jobData); err != nil {
		return fmt.Errorf("removeDeprecatedFields failed: %w", err)
	}

	return nil
}

// processJob reads, migrates, and writes a job metadata file
func processJob(metaPath string, dryRun bool) error {
	// Read the meta.json file
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", metaPath, err)
	}

	// Parse JSON
	var jobData map[string]interface{}
	if err := json.Unmarshal(data, &jobData); err != nil {
		return fmt.Errorf("failed to parse JSON from %s: %w", metaPath, err)
	}

	// Apply migrations
	if err := migrateJobMetadata(jobData); err != nil {
		return fmt.Errorf("migration failed for %s: %w", metaPath, err)
	}

	// If dry-run, just report what would change
	if dryRun {
		cclog.Infof("Would migrate: %s", metaPath)
		return nil
	}

	// Write back the migrated data
	migratedData, err := json.MarshalIndent(jobData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal migrated data: %w", err)
	}

	if err := os.WriteFile(metaPath, migratedData, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", metaPath, err)
	}

	return nil
}

// migrateArchive walks through an archive directory and migrates all meta.json files
func migrateArchive(archivePath string, dryRun bool, numWorkers int) (int, int, error) {
	cclog.Infof("Starting archive migration at %s", archivePath)
	if dryRun {
		cclog.Info("DRY RUN MODE - no files will be modified")
	}

	var migrated int32
	var failed int32

	// Channel for job paths
	jobs :=make(chan string, numWorkers*2)
	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for metaPath := range jobs {
				if err := processJob(metaPath, dryRun); err != nil {
					cclog.Errorf("Failed to migrate %s: %s", metaPath, err.Error())
					atomic.AddInt32(&failed, 1)
					continue
				}

				newCount := atomic.AddInt32(&migrated, 1)
				if newCount%100 == 0 {
					cclog.Infof("Progress: %d jobs migrated, %d failed", newCount, atomic.LoadInt32(&failed))
				}
			}
		}(i)
	}

	// Walk the archive directory and find all meta.json files
	go func() {
		filepath.Walk(archivePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				cclog.Errorf("Error accessing path %s: %s", path, err.Error())
				return nil // Continue walking
			}

			if !info.IsDir() && info.Name() == "meta.json" {
				jobs <- path
			}

			return nil
		})
		close(jobs)
	}()

	// Wait for all workers to complete
	wg.Wait()

	finalMigrated := int(atomic.LoadInt32(&migrated))
	finalFailed := int(atomic.LoadInt32(&failed))

	cclog.Infof("Migration completed: %d jobs migrated, %d failed", finalMigrated, finalFailed)

	if finalFailed > 0 {
		return finalMigrated, finalFailed, fmt.Errorf("%d jobs failed to migrate", finalFailed)
	}

	return finalMigrated, finalFailed, nil
}
