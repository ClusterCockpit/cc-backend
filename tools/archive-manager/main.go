// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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

// countJobs counts the total number of jobs in the source archive using external fd command.
// It requires the fd binary to be available in PATH.
// The srcConfig parameter should be the JSON configuration string containing the archive path.
func countJobs(srcConfig string) (int, error) {
	fdPath, err := exec.LookPath("fd")
	if err != nil {
		return 0, fmt.Errorf("fd binary not found in PATH: %w", err)
	}

	var config struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(srcConfig), &config); err != nil {
		return 0, fmt.Errorf("failed to parse source config: %w", err)
	}

	if config.Path == "" {
		return 0, fmt.Errorf("no path found in source config")
	}

	fdCmd := exec.Command(fdPath, "meta.json", config.Path)
	wcCmd := exec.Command("wc", "-l")

	pipe, err := fdCmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create pipe: %w", err)
	}
	wcCmd.Stdin = pipe

	if err := fdCmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start fd command: %w", err)
	}

	output, err := wcCmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run wc command: %w", err)
	}

	if err := fdCmd.Wait(); err != nil {
		return 0, fmt.Errorf("fd command failed: %w", err)
	}

	countStr := strings.TrimSpace(string(output))
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse count from wc output '%s': %w", countStr, err)
	}

	return count, nil
}

// formatDuration formats a duration as a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// progressMeter displays import progress to the terminal.
type progressMeter struct {
	total     int
	processed int32
	imported  int32
	skipped   int32
	failed    int32
	startTime time.Time
	done      chan struct{}
}

func newProgressMeter(total int) *progressMeter {
	return &progressMeter{
		total:     total,
		startTime: time.Now(),
		done:      make(chan struct{}),
	}
}

func (p *progressMeter) start() {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				p.render()
			case <-p.done:
				p.render()
				fmt.Println()
				return
			}
		}
	}()
}

func (p *progressMeter) render() {
	processed := atomic.LoadInt32(&p.processed)
	imported := atomic.LoadInt32(&p.imported)
	skipped := atomic.LoadInt32(&p.skipped)
	failed := atomic.LoadInt32(&p.failed)

	elapsed := time.Since(p.startTime)
	percent := float64(processed) / float64(p.total) * 100
	if p.total == 0 {
		percent = 0
	}

	var eta string
	var throughput float64
	if processed > 0 {
		throughput = float64(processed) / elapsed.Seconds()
		remaining := float64(p.total-int(processed)) / throughput
		eta = formatDuration(time.Duration(remaining) * time.Second)
	} else {
		eta = "calculating..."
	}

	barWidth := 30
	filled := int(float64(barWidth) * float64(processed) / float64(p.total))
	if p.total == 0 {
		filled = 0
	}

	var bar strings.Builder
	for i := range barWidth {
		if i < filled {
			bar.WriteString("█")
		} else {
			bar.WriteString("░")
		}
	}

	fmt.Printf("\r[%s] %5.1f%% | %d/%d | %.1f jobs/s | ETA: %s | ✓%d ○%d ✗%d   ",
		bar.String(), percent, processed, p.total, throughput, eta, imported, skipped, failed)
}

func (p *progressMeter) stop() {
	close(p.done)
}

// importArchive imports all jobs from a source archive backend to a destination archive backend.
// It uses parallel processing with a worker pool to improve performance.
// The import can be interrupted by CTRL-C (SIGINT) and will terminate gracefully.
// Returns the number of successfully imported jobs, failed jobs, and any error encountered.
func importArchive(srcBackend, dstBackend archive.ArchiveBackend, srcConfig string) (int, int, error) {
	cclog.Info("Starting parallel archive import...")
	cclog.Info("Press CTRL-C to interrupt (will finish current jobs before exiting)")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var interrupted atomic.Bool

	go func() {
		<-sigChan
		cclog.Warn("Interrupt received, stopping import (finishing current jobs)...")
		interrupted.Store(true)
		cancel()
		// Stop listening for further signals to allow force quit with second CTRL-C
		signal.Stop(sigChan)
	}()

	cclog.Info("Counting jobs in source archive (this may take a long time) ...")
	totalJobs, err := countJobs(srcConfig)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count jobs: %w", err)
	}
	cclog.Infof("Found %d jobs to process", totalJobs)

	progress := newProgressMeter(totalJobs)

	numWorkers := 4
	cclog.Infof("Using %d parallel workers", numWorkers)

	jobs := make(chan archive.JobContainer, numWorkers*2)

	var wg sync.WaitGroup

	progress.start()

	for i := range numWorkers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for job := range jobs {
				if job.Meta == nil {
					cclog.Warn("Skipping job with nil metadata")
					atomic.AddInt32(&progress.failed, 1)
					atomic.AddInt32(&progress.processed, 1)
					continue
				}

				if job.Data == nil {
					cclog.Warnf("Job %d from cluster %s has no metric data, skipping",
						job.Meta.JobID, job.Meta.Cluster)
					atomic.AddInt32(&progress.failed, 1)
					atomic.AddInt32(&progress.processed, 1)
					continue
				}

				if dstBackend.Exists(job.Meta) {
					cclog.Debugf("Job %d (cluster: %s, start: %d) already exists in destination, skipping",
						job.Meta.JobID, job.Meta.Cluster, job.Meta.StartTime)
					atomic.AddInt32(&progress.skipped, 1)
					atomic.AddInt32(&progress.processed, 1)
					continue
				}

				if err := dstBackend.ImportJob(job.Meta, job.Data); err != nil {
					cclog.Errorf("Failed to import job %d from cluster %s: %s",
						job.Meta.JobID, job.Meta.Cluster, err.Error())
					atomic.AddInt32(&progress.failed, 1)
					atomic.AddInt32(&progress.processed, 1)
					continue
				}

				atomic.AddInt32(&progress.imported, 1)
				atomic.AddInt32(&progress.processed, 1)
			}
		}(i)
	}

	go func() {
		defer close(jobs)

		clusters := srcBackend.GetClusters()
		for _, clusterName := range clusters {
			if ctx.Err() != nil {
				return
			}

			clusterCfg, err := srcBackend.LoadClusterCfg(clusterName)
			if err != nil {
				cclog.Errorf("Failed to load cluster config for %s: %v", clusterName, err)
				continue
			}

			if err := dstBackend.StoreClusterCfg(clusterName, clusterCfg); err != nil {
				cclog.Errorf("Failed to store cluster config for %s: %v", clusterName, err)
			} else {
				cclog.Infof("Imported cluster config for %s", clusterName)
			}
		}

		for job := range srcBackend.Iter(true) {
			select {
			case <-ctx.Done():
				// Drain remaining items from iterator to avoid resource leak
				// but don't process them
				return
			case jobs <- job:
			}
		}
	}()

	wg.Wait()
	progress.stop()

	finalImported := int(atomic.LoadInt32(&progress.imported))
	finalFailed := int(atomic.LoadInt32(&progress.failed))
	finalSkipped := int(atomic.LoadInt32(&progress.skipped))

	elapsed := time.Since(progress.startTime)

	if interrupted.Load() {
		cclog.Warnf("Import interrupted after %s: %d jobs imported, %d skipped, %d failed",
			formatDuration(elapsed), finalImported, finalSkipped, finalFailed)
		return finalImported, finalFailed, fmt.Errorf("import interrupted by user")
	}

	cclog.Infof("Import completed in %s: %d jobs imported, %d skipped, %d failed",
		formatDuration(elapsed), finalImported, finalSkipped, finalFailed)

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
	flag.StringVar(&flagLogLevel, "loglevel", "info", "Sets the logging level: `[debug,info,warn (default),err,fatal,crit]`")
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
		imported, failed, err := importArchive(srcBackend, dstBackend, flagSrcConfig)
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
			cclog.Debugf("Validate %s - %d", job.Meta.Cluster, job.Meta.JobID)
		}
		os.Exit(0)
	}

	if flagRemoveBefore != "" || flagRemoveAfter != "" {
		ar.Clean(parseDate(flagRemoveBefore), parseDate(flagRemoveAfter))
		os.Exit(0)
	}

	ar.Info()
}
