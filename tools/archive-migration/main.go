// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"flag"
	"fmt"
	"os"

	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
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

	// Display warning for non-dry-run mode
	if !dryRun {
		cclog.Warn("WARNING: This will modify files in the archive!")
		cclog.Warn("It is strongly recommended to backup your archive first.")
		cclog.Warn("Run with --dry-run first to preview changes.")
		cclog.Info("")
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
		cclog.Infof("Migration completed successfully: %d jobs migrated", migrated)
	}
}
