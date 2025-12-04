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
	"testing"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"
)

// TestImportFileToSqlite tests importing jobs from file backend to SQLite backend
func TestImportFileToSqlite(t *testing.T) {
	// Create temporary directories
	tmpdir := t.TempDir()
	srcArchive := filepath.Join(tmpdir, "src-archive")
	dstDb := filepath.Join(tmpdir, "dst-archive.db")

	// Copy test data to source archive
	testDataPath := "../../pkg/archive/testdata/archive"
	if _, err := os.Stat(testDataPath); os.IsNotExist(err) {
		t.Skip("Test data not found, skipping integration test")
	}

	if err := util.CopyDir(testDataPath, srcArchive); err != nil {
		t.Fatalf("Failed to copy test data: %s", err.Error())
	}

	// Initialize source backend (file)
	srcConfig := fmt.Sprintf(`{"kind":"file","path":"%s"}`, srcArchive)
	srcBackend, err := archive.InitBackend(json.RawMessage(srcConfig))
	if err != nil {
		t.Fatalf("Failed to initialize source backend: %s", err.Error())
	}

	// Initialize destination backend (sqlite)
	dstConfig := fmt.Sprintf(`{"kind":"sqlite","dbPath":"%s"}`, dstDb)
	dstBackend, err := archive.InitBackend(json.RawMessage(dstConfig))
	if err != nil {
		t.Fatalf("Failed to initialize destination backend: %s", err.Error())
	}

	// Perform import
	imported, failed, err := importArchive(srcBackend, dstBackend)
	if err != nil {
		t.Errorf("Import failed: %s", err.Error())
	}

	if imported == 0 {
		t.Error("No jobs were imported")
	}

	if failed > 0 {
		t.Errorf("%d jobs failed to import", failed)
	}

	t.Logf("Successfully imported %d jobs", imported)

	// Verify jobs exist in destination
	// Count jobs in source
	srcCount := 0
	for range srcBackend.Iter(false) {
		srcCount++
	}

	// Count jobs in destination
	dstCount := 0
	for range dstBackend.Iter(false) {
		dstCount++
	}

	if srcCount != dstCount {
		t.Errorf("Job count mismatch: source has %d jobs, destination has %d jobs", srcCount, dstCount)
	}
}

// TestImportFileToFile tests importing jobs from one file backend to another
func TestImportFileToFile(t *testing.T) {
	// Create temporary directories
	tmpdir := t.TempDir()
	srcArchive := filepath.Join(tmpdir, "src-archive")
	dstArchive := filepath.Join(tmpdir, "dst-archive")

	// Copy test data to source archive
	testDataPath := "../../pkg/archive/testdata/archive"
	if _, err := os.Stat(testDataPath); os.IsNotExist(err) {
		t.Skip("Test data not found, skipping integration test")
	}

	if err := util.CopyDir(testDataPath, srcArchive); err != nil {
		t.Fatalf("Failed to copy test data: %s", err.Error())
	}

	// Create destination archive directory
	if err := os.MkdirAll(dstArchive, 0755); err != nil {
		t.Fatalf("Failed to create destination directory: %s", err.Error())
	}

	// Write version file
	versionFile := filepath.Join(dstArchive, "version.txt")
	if err := os.WriteFile(versionFile, []byte("3"), 0644); err != nil {
		t.Fatalf("Failed to write version file: %s", err.Error())
	}

	// Initialize source backend
	srcConfig := fmt.Sprintf(`{"kind":"file","path":"%s"}`, srcArchive)
	srcBackend, err := archive.InitBackend(json.RawMessage(srcConfig))
	if err != nil {
		t.Fatalf("Failed to initialize source backend: %s", err.Error())
	}

	// Initialize destination backend
	dstConfig := fmt.Sprintf(`{"kind":"file","path":"%s"}`, dstArchive)
	dstBackend, err := archive.InitBackend(json.RawMessage(dstConfig))
	if err != nil {
		t.Fatalf("Failed to initialize destination backend: %s", err.Error())
	}

	// Perform import
	imported, failed, err := importArchive(srcBackend, dstBackend)
	if err != nil {
		t.Errorf("Import failed: %s", err.Error())
	}

	if imported == 0 {
		t.Error("No jobs were imported")
	}

	if failed > 0 {
		t.Errorf("%d jobs failed to import", failed)
	}

	t.Logf("Successfully imported %d jobs", imported)
}

// TestImportDataIntegrity verifies that job metadata and data are correctly imported
func TestImportDataIntegrity(t *testing.T) {
	// Create temporary directories
	tmpdir := t.TempDir()
	srcArchive := filepath.Join(tmpdir, "src-archive")
	dstDb := filepath.Join(tmpdir, "dst-archive.db")

	// Copy test data to source archive
	testDataPath := "../../pkg/archive/testdata/archive"
	if _, err := os.Stat(testDataPath); os.IsNotExist(err) {
		t.Skip("Test data not found, skipping integration test")
	}

	if err := util.CopyDir(testDataPath, srcArchive); err != nil {
		t.Fatalf("Failed to copy test data: %s", err.Error())
	}

	// Initialize backends
	srcConfig := fmt.Sprintf(`{"kind":"file","path":"%s"}`, srcArchive)
	srcBackend, err := archive.InitBackend(json.RawMessage(srcConfig))
	if err != nil {
		t.Fatalf("Failed to initialize source backend: %s", err.Error())
	}

	dstConfig := fmt.Sprintf(`{"kind":"sqlite","dbPath":"%s"}`, dstDb)
	dstBackend, err := archive.InitBackend(json.RawMessage(dstConfig))
	if err != nil {
		t.Fatalf("Failed to initialize destination backend: %s", err.Error())
	}

	// Perform import
	_, _, err = importArchive(srcBackend, dstBackend)
	if err != nil {
		t.Errorf("Import failed: %s", err.Error())
	}

	// Verify data integrity for each job
	verifiedJobs := 0
	for srcJob := range srcBackend.Iter(false) {
		if srcJob.Meta == nil {
			continue
		}

		// Load job from destination
		dstJobMeta, err := dstBackend.LoadJobMeta(srcJob.Meta)
		if err != nil {
			t.Errorf("Failed to load job %d from destination: %s", srcJob.Meta.JobID, err.Error())
			continue
		}

		// Verify basic metadata
		if dstJobMeta.JobID != srcJob.Meta.JobID {
			t.Errorf("JobID mismatch: expected %d, got %d", srcJob.Meta.JobID, dstJobMeta.JobID)
		}

		if dstJobMeta.Cluster != srcJob.Meta.Cluster {
			t.Errorf("Cluster mismatch for job %d: expected %s, got %s",
				srcJob.Meta.JobID, srcJob.Meta.Cluster, dstJobMeta.Cluster)
		}

		if dstJobMeta.StartTime != srcJob.Meta.StartTime {
			t.Errorf("StartTime mismatch for job %d: expected %d, got %d",
				srcJob.Meta.JobID, srcJob.Meta.StartTime, dstJobMeta.StartTime)
		}

		// Load and verify job data
		srcData, err := srcBackend.LoadJobData(srcJob.Meta)
		if err != nil {
			t.Errorf("Failed to load job data from source: %s", err.Error())
			continue
		}

		dstData, err := dstBackend.LoadJobData(srcJob.Meta)
		if err != nil {
			t.Errorf("Failed to load job data from destination: %s", err.Error())
			continue
		}

		// Verify metric data exists
		if len(srcData) != len(dstData) {
			t.Errorf("Metric count mismatch for job %d: expected %d, got %d",
				srcJob.Meta.JobID, len(srcData), len(dstData))
		}

		verifiedJobs++
	}

	if verifiedJobs == 0 {
		t.Error("No jobs were verified")
	}

	t.Logf("Successfully verified %d jobs", verifiedJobs)
}

// TestImportEmptyArchive tests importing from an empty archive
func TestImportEmptyArchive(t *testing.T) {
	tmpdir := t.TempDir()
	srcArchive := filepath.Join(tmpdir, "empty-archive")
	dstDb := filepath.Join(tmpdir, "dst-archive.db")

	// Create empty source archive
	if err := os.MkdirAll(srcArchive, 0755); err != nil {
		t.Fatalf("Failed to create source directory: %s", err.Error())
	}

	// Write version file
	versionFile := filepath.Join(srcArchive, "version.txt")
	if err := os.WriteFile(versionFile, []byte("3"), 0644); err != nil {
		t.Fatalf("Failed to write version file: %s", err.Error())
	}

	// Initialize backends
	srcConfig := fmt.Sprintf(`{"kind":"file","path":"%s"}`, srcArchive)
	srcBackend, err := archive.InitBackend(json.RawMessage(srcConfig))
	if err != nil {
		t.Fatalf("Failed to initialize source backend: %s", err.Error())
	}

	dstConfig := fmt.Sprintf(`{"kind":"sqlite","dbPath":"%s"}`, dstDb)
	dstBackend, err := archive.InitBackend(json.RawMessage(dstConfig))
	if err != nil {
		t.Fatalf("Failed to initialize destination backend: %s", err.Error())
	}

	// Perform import
	imported, failed, err := importArchive(srcBackend, dstBackend)
	if err != nil {
		t.Errorf("Import from empty archive should not fail: %s", err.Error())
	}

	if imported != 0 {
		t.Errorf("Expected 0 imported jobs, got %d", imported)
	}

	if failed != 0 {
		t.Errorf("Expected 0 failed jobs, got %d", failed)
	}
}

// TestImportDuplicateJobs tests that duplicate jobs are skipped
func TestImportDuplicateJobs(t *testing.T) {
	tmpdir := t.TempDir()
	srcArchive := filepath.Join(tmpdir, "src-archive")
	dstDb := filepath.Join(tmpdir, "dst-archive.db")

	// Copy test data
	testDataPath := "../../pkg/archive/testdata/archive"
	if _, err := os.Stat(testDataPath); os.IsNotExist(err) {
		t.Skip("Test data not found, skipping integration test")
	}

	if err := util.CopyDir(testDataPath, srcArchive); err != nil {
		t.Fatalf("Failed to copy test data: %s", err.Error())
	}

	// Initialize backends
	srcConfig := fmt.Sprintf(`{"kind":"file","path":"%s"}`, srcArchive)
	srcBackend, err := archive.InitBackend(json.RawMessage(srcConfig))
	if err != nil {
		t.Fatalf("Failed to initialize source backend: %s", err.Error())
	}

	dstConfig := fmt.Sprintf(`{"kind":"sqlite","dbPath":"%s"}`, dstDb)
	dstBackend, err := archive.InitBackend(json.RawMessage(dstConfig))
	if err != nil {
		t.Fatalf("Failed to initialize destination backend: %s", err.Error())
	}

	// First import
	imported1, _, err := importArchive(srcBackend, dstBackend)
	if err != nil {
		t.Fatalf("First import failed: %s", err.Error())
	}

	// Second import (should skip all jobs)
	imported2, _, err := importArchive(srcBackend, dstBackend)
	if err != nil {
		t.Errorf("Second import failed: %s", err.Error())
	}

	if imported2 != 0 {
		t.Errorf("Second import should skip all jobs, but imported %d", imported2)
	}

	t.Logf("First import: %d jobs, Second import: %d jobs (all skipped as expected)", imported1, imported2)
}

// TestJobStub is a helper test to verify that the job stub used in tests matches the schema
func TestJobStub(t *testing.T) {
	job := &schema.Job{
		JobID:     123,
		Cluster:   "test-cluster",
		StartTime: 1234567890,
	}

	if job.JobID != 123 {
		t.Errorf("Expected JobID 123, got %d", job.JobID)
	}
}
