// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// Worker for either Archiving or Deleting files

func CleanUp(wg *sync.WaitGroup, ctx context.Context) {
	if Keys.Cleanup.Mode == "archive" {
		// Run as Archiver
		cleanUpWorker(wg, ctx,
			Keys.RetentionInMemory,
			"archiving",
			Keys.Cleanup.RootDir,
			false,
		)
	} else {
		// Run as Deleter
		cleanUpWorker(wg, ctx,
			Keys.RetentionInMemory,
			"deleting",
			"",
			true,
		)
	}
}

// cleanUpWorker takes simple values to configure what it does
func cleanUpWorker(wg *sync.WaitGroup, ctx context.Context, interval string, mode string, cleanupDir string, delete bool) {
	wg.Go(func() {

		d, err := time.ParseDuration(interval)
		if err != nil {
			cclog.Fatalf("[METRICSTORE]> error parsing %s interval duration: %v\n", mode, err)
		}
		if d <= 0 {
			return
		}

		ticker := time.NewTicker(d)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t := time.Now().Add(-d)
				cclog.Infof("[METRICSTORE]> start %s checkpoints (older than %s)...", mode, t.Format(time.RFC3339))

				n, err := CleanupCheckpoints(Keys.Checkpoints.RootDir, cleanupDir, t.Unix(), delete)

				if err != nil {
					cclog.Errorf("[METRICSTORE]> %s failed: %s", mode, err.Error())
				} else {
					if delete {
						cclog.Infof("[METRICSTORE]> done: %d checkpoints deleted", n)
					} else {
						cclog.Infof("[METRICSTORE]> done: %d checkpoint files archived to parquet", n)
					}
				}
			}
		}
	})
}

var ErrNoNewArchiveData error = errors.New("all data already archived")

// CleanupCheckpoints deletes or archives all checkpoint files older than `from`.
// When archiving, consolidates all hosts per cluster into a single Parquet file.
func CleanupCheckpoints(checkpointsDir, cleanupDir string, from int64, deleteInstead bool) (int, error) {
	if deleteInstead {
		return deleteCheckpoints(checkpointsDir, from)
	}

	return archiveCheckpoints(checkpointsDir, cleanupDir, from)
}

// deleteCheckpoints removes checkpoint files older than `from` across all clusters/hosts.
func deleteCheckpoints(checkpointsDir string, from int64) (int, error) {
	entries1, err := os.ReadDir(checkpointsDir)
	if err != nil {
		return 0, err
	}

	type workItem struct {
		dir            string
		cluster, host  string
	}

	var wg sync.WaitGroup
	n, errs := int32(0), int32(0)
	work := make(chan workItem, Keys.NumWorkers)

	wg.Add(Keys.NumWorkers)
	for worker := 0; worker < Keys.NumWorkers; worker++ {
		go func() {
			defer wg.Done()
			for item := range work {
				entries, err := os.ReadDir(item.dir)
				if err != nil {
					cclog.Errorf("error reading %s/%s: %s", item.cluster, item.host, err.Error())
					atomic.AddInt32(&errs, 1)
					continue
				}

				files, err := findFiles(entries, from, false)
				if err != nil {
					cclog.Errorf("error finding files in %s/%s: %s", item.cluster, item.host, err.Error())
					atomic.AddInt32(&errs, 1)
					continue
				}

				for _, checkpoint := range files {
					if err := os.Remove(filepath.Join(item.dir, checkpoint)); err != nil {
						cclog.Errorf("error deleting %s/%s/%s: %s", item.cluster, item.host, checkpoint, err.Error())
						atomic.AddInt32(&errs, 1)
					} else {
						atomic.AddInt32(&n, 1)
					}
				}
			}
		}()
	}

	for _, de1 := range entries1 {
		entries2, e := os.ReadDir(filepath.Join(checkpointsDir, de1.Name()))
		if e != nil {
			err = e
			continue
		}

		for _, de2 := range entries2 {
			work <- workItem{
				dir:     filepath.Join(checkpointsDir, de1.Name(), de2.Name()),
				cluster: de1.Name(),
				host:    de2.Name(),
			}
		}
	}

	close(work)
	wg.Wait()

	if err != nil {
		return int(n), err
	}
	if errs > 0 {
		return int(n), fmt.Errorf("%d errors happened while deleting (%d successes)", errs, n)
	}
	return int(n), nil
}

// archiveCheckpoints archives checkpoint files to Parquet format.
// Produces one Parquet file per cluster: <cleanupDir>/<cluster>/<timestamp>.parquet
// Workers load checkpoint files from disk and send CheckpointFile trees on a
// back-pressured channel. The main thread streams each tree directly to Parquet
// rows without materializing all rows in memory.
func archiveCheckpoints(checkpointsDir, cleanupDir string, from int64) (int, error) {
	cclog.Info("[METRICSTORE]> start archiving checkpoints to parquet")
	startTime := time.Now()

	clusterEntries, err := os.ReadDir(checkpointsDir)
	if err != nil {
		return 0, err
	}

	totalFiles := 0

	for _, clusterEntry := range clusterEntries {
		if !clusterEntry.IsDir() {
			continue
		}

		cluster := clusterEntry.Name()
		hostEntries, err := os.ReadDir(filepath.Join(checkpointsDir, cluster))
		if err != nil {
			return totalFiles, err
		}

		// Workers load checkpoint files from disk; main thread writes to parquet.
		type hostResult struct {
			checkpoints []*CheckpointFile
			hostname    string
			files       []string // checkpoint filenames to delete after successful write
			dir         string   // checkpoint directory for this host
		}

		// Small buffer provides back-pressure: at most NumWorkers+2 results in flight.
		results := make(chan hostResult, 2)
		work := make(chan struct {
			dir, host string
		}, Keys.NumWorkers)

		var wg sync.WaitGroup
		errs := int32(0)

		wg.Add(Keys.NumWorkers)
		for w := 0; w < Keys.NumWorkers; w++ {
			go func() {
				defer wg.Done()
				for item := range work {
					checkpoints, files, err := loadCheckpointFiles(item.dir, from)
					if err != nil {
						cclog.Errorf("[METRICSTORE]> error reading checkpoints for %s/%s: %s", cluster, item.host, err.Error())
						atomic.AddInt32(&errs, 1)
						continue
					}
					if len(checkpoints) > 0 {
						results <- hostResult{
							checkpoints: checkpoints,
							hostname:    item.host,
							files:       files,
							dir:         item.dir,
						}
					}
				}
			}()
		}

		go func() {
			for _, hostEntry := range hostEntries {
				if !hostEntry.IsDir() {
					continue
				}
				dir := filepath.Join(checkpointsDir, cluster, hostEntry.Name())
				work <- struct {
					dir, host string
				}{dir: dir, host: hostEntry.Name()}
			}
			close(work)
			wg.Wait()
			close(results)
		}()

		// Open streaming writer and write each host's checkpoint files as a row group
		parquetFile := filepath.Join(cleanupDir, cluster, fmt.Sprintf("%d.parquet", from))
		writer, err := newParquetArchiveWriter(parquetFile)
		if err != nil {
			// Drain results channel to unblock workers
			for range results {
			}
			return totalFiles, fmt.Errorf("creating parquet writer for cluster %s: %w", cluster, err)
		}

		type deleteItem struct {
			dir   string
			files []string
		}
		var toDelete []deleteItem
		writeErr := error(nil)

		for r := range results {
			if writeErr == nil {
				// Stream each checkpoint file directly to parquet rows.
				// Each checkpoint is processed and discarded before the next.
				for _, cf := range r.checkpoints {
					if err := writer.WriteCheckpointFile(cf, cluster, r.hostname, "node", ""); err != nil {
						writeErr = err
						break
					}
				}
			}
			// Always track files for deletion (even if write failed, we still drain)
			toDelete = append(toDelete, deleteItem{dir: r.dir, files: r.files})
		}

		if err := writer.Close(); err != nil && writeErr == nil {
			writeErr = err
		}

		if errs > 0 {
			return totalFiles, fmt.Errorf("%d errors reading checkpoints for cluster %s", errs, cluster)
		}

		if writer.count == 0 {
			// No data written — remove empty file
			os.Remove(parquetFile)
			continue
		}

		if writeErr != nil {
			os.Remove(parquetFile)
			return totalFiles, fmt.Errorf("writing parquet archive for cluster %s: %w", cluster, writeErr)
		}

		// Delete archived checkpoint files
		for _, item := range toDelete {
			for _, file := range item.files {
				filename := filepath.Join(item.dir, file)
				if err := os.Remove(filename); err != nil {
					cclog.Warnf("[METRICSTORE]> could not remove archived checkpoint %s: %v", filename, err)
				} else {
					totalFiles++
				}
			}
		}

		cclog.Infof("[METRICSTORE]> archived %d rows from %d files for cluster %s to %s",
			writer.count, totalFiles, cluster, parquetFile)
	}

	cclog.Infof("[METRICSTORE]> archiving checkpoints completed in %s (%d files)", time.Since(startTime).Round(time.Millisecond), totalFiles)
	return totalFiles, nil
}
