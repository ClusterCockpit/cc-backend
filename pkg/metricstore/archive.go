// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
			Keys.Cleanup.Interval,
			"archiving",
			Keys.Cleanup.RootDir,
			false,
		)
	} else {
		if Keys.Cleanup.Interval == "" {
			Keys.Cleanup.Interval = Keys.RetentionInMemory
		}

		// Run as Deleter
		cleanUpWorker(wg, ctx,
			Keys.Cleanup.Interval,
			"deleting",
			"",
			true,
		)
	}
}

// runWorker takes simple values to configure what it does
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
					if delete && cleanupDir == "" {
						cclog.Infof("[METRICSTORE]> done: %d checkpoints deleted", n)
					} else {
						cclog.Infof("[METRICSTORE]> done: %d files zipped and moved to archive", n)
					}
				}
			}
		}
	})
}

var ErrNoNewArchiveData error = errors.New("all data already archived")

// Delete or ZIP all checkpoint files older than `from` together and write them to the `cleanupDir`,
// deleting/moving them from the `checkpointsDir`.
func CleanupCheckpoints(checkpointsDir, cleanupDir string, from int64, deleteInstead bool) (int, error) {
	entries1, err := os.ReadDir(checkpointsDir)
	if err != nil {
		return 0, err
	}

	type workItem struct {
		cdir, adir    string
		cluster, host string
	}

	var wg sync.WaitGroup
	n, errs := int32(0), int32(0)
	work := make(chan workItem, Keys.NumWorkers)

	wg.Add(Keys.NumWorkers)
	for worker := 0; worker < Keys.NumWorkers; worker++ {
		go func() {
			defer wg.Done()
			for workItem := range work {
				m, err := cleanupCheckpoints(workItem.cdir, workItem.adir, from, deleteInstead)
				if err != nil {
					cclog.Errorf("error while archiving %s/%s: %s", workItem.cluster, workItem.host, err.Error())
					atomic.AddInt32(&errs, 1)
				}
				atomic.AddInt32(&n, int32(m))
			}
		}()
	}

	for _, de1 := range entries1 {
		entries2, e := os.ReadDir(filepath.Join(checkpointsDir, de1.Name()))
		if e != nil {
			err = e
		}

		for _, de2 := range entries2 {
			cdir := filepath.Join(checkpointsDir, de1.Name(), de2.Name())
			adir := filepath.Join(cleanupDir, de1.Name(), de2.Name())
			work <- workItem{
				adir: adir, cdir: cdir,
				cluster: de1.Name(), host: de2.Name(),
			}
		}
	}

	close(work)
	wg.Wait()

	if err != nil {
		return int(n), err
	}

	if errs > 0 {
		return int(n), fmt.Errorf("%d errors happened while archiving (%d successes)", errs, n)
	}
	return int(n), nil
}

// Helper function for `CleanupCheckpoints`.
func cleanupCheckpoints(dir string, cleanupDir string, from int64, deleteInstead bool) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	files, err := findFiles(entries, from, false)
	if err != nil {
		return 0, err
	}

	if deleteInstead {
		n := 0
		for _, checkpoint := range files {
			filename := filepath.Join(dir, checkpoint)
			if err = os.Remove(filename); err != nil {
				return n, err
			}
			n += 1
		}
		return n, nil
	}

	filename := filepath.Join(cleanupDir, fmt.Sprintf("%d.zip", from))
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, CheckpointFilePerms)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(cleanupDir, CheckpointDirPerms)
		if err == nil {
			f, err = os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, CheckpointFilePerms)
		}
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	defer bw.Flush()
	zw := zip.NewWriter(bw)
	defer zw.Close()

	n := 0
	for _, checkpoint := range files {
		// Use closure to ensure file is closed immediately after use,
		// avoiding file descriptor leak from defer in loop
		err := func() error {
			filename := filepath.Join(dir, checkpoint)
			r, err := os.Open(filename)
			if err != nil {
				return err
			}
			defer r.Close()

			w, err := zw.Create(checkpoint)
			if err != nil {
				return err
			}

			if _, err = io.Copy(w, r); err != nil {
				return err
			}

			if err = os.Remove(filename); err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			return n, err
		}
		n += 1
	}

	return n, nil
}
