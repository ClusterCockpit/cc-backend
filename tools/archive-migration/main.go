// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

func main() {
	var archivePath string
	var dryRun bool
	var numWorkers int
	var flagLogLevel string
	var flagLogDateTime bool

	flag.StringVar(&archivePath, "archive", "", "Path to job archive to migrate (required)")
	flag.BoolVar(&dryRun, "dry-run", false, "Preview changes without modifying files")
	flag.IntVar(&numWorkers, "workers", 4, "Number of parallel workers")
	flag.StringVar(&flagLogLevel, "loglevel", "info", "Sets the logging level: `[debug,info,warn (default),err,fatal,crit]`")
	flag.BoolVar(&flagLogDateTime, "logdate", false, "Add date and time to log messages")
	flag.Parse()

	// Initialize logger
	cclog.Init(flagLogLevel, flagLogDateTime)

	// Validate inputs
	if archivePath == "" {
		fmt.Fprintf(os.Stderr, "Error: --archive flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Check if archive path exists
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		cclog.Fatalf("Archive path does not exist: %s", archivePath)
	}

	// Check archive version
	if err := checkVersion(archivePath); err != nil {
		cclog.Fatalf("Version check failed: %v", err)
	}

	// Display warning for non-dry-run mode
	if !dryRun {
		cclog.Warn("WARNING: This will modify files in the archive!")
		cclog.Warn("It is strongly recommended to backup your archive first.")
		cclog.Warn("Run with --dry-run first to preview changes.")
		cclog.Info("")

		fmt.Print("Are you sure you want to continue? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			cclog.Fatalf("Error reading input: %v", err)
		}
		if strings.ToLower(strings.TrimSpace(input)) != "y" {
			cclog.Info("Aborted by user.")
			os.Exit(0)
		}
	}

	// Run migration
	migrated, failed, err := migrateArchive(archivePath, dryRun, numWorkers)

	if err != nil {
		cclog.Errorf("Migration completed with errors: %s", err.Error())
		if failed > 0 {
			os.Exit(1)
		}
	}

	if dryRun {
		cclog.Infof("Dry run completed: %d jobs would be migrated", migrated)
	} else {
		if err := updateVersion(archivePath); err != nil {
			cclog.Errorf("Failed to update archive version: %v", err)
			os.Exit(1)
		}
		cclog.Infof("Migration completed successfully: %d jobs migrated", migrated)
	}
}

func checkVersion(archivePath string) error {
	versionFile := filepath.Join(archivePath, "version.txt")
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return fmt.Errorf("failed to read version.txt: %v", err)
	}
	versionStr := strings.TrimSpace(string(data))
	if versionStr != "2" {
		return fmt.Errorf("archive version is %s, expected 2", versionStr)
	}
	return nil
}

func updateVersion(archivePath string) error {
	versionFile := filepath.Join(archivePath, "version.txt")
	return os.WriteFile(versionFile, []byte("3\n"), 0644)
}
