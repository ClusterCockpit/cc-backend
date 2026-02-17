// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	pqarchive "github.com/ClusterCockpit/cc-backend/pkg/archive/parquet"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/go-co-op/gocron/v2"
)

// createParquetTarget creates a ParquetTarget (file or S3) from the retention config.
func createParquetTarget(cfg Retention) (pqarchive.ParquetTarget, error) {
	switch cfg.TargetKind {
	case "s3":
		return pqarchive.NewS3Target(pqarchive.S3TargetConfig{
			Endpoint:     cfg.TargetEndpoint,
			Bucket:       cfg.TargetBucket,
			AccessKey:    cfg.TargetAccessKey,
			SecretKey:    cfg.TargetSecretKey,
			Region:       cfg.TargetRegion,
			UsePathStyle: cfg.TargetUsePathStyle,
		})
	default:
		return pqarchive.NewFileTarget(cfg.TargetPath)
	}
}

// createTargetBackend creates a secondary archive backend (file or S3) for JSON copy/move.
func createTargetBackend(cfg Retention) (archive.ArchiveBackend, error) {
	var raw json.RawMessage
	var err error

	switch cfg.TargetKind {
	case "s3":
		raw, err = json.Marshal(map[string]any{
			"kind":           "s3",
			"endpoint":       cfg.TargetEndpoint,
			"bucket":         cfg.TargetBucket,
			"access-key":     cfg.TargetAccessKey,
			"secret-key":     cfg.TargetSecretKey,
			"region":         cfg.TargetRegion,
			"use-path-style": cfg.TargetUsePathStyle,
		})
	default:
		raw, err = json.Marshal(map[string]string{
			"kind": "file",
			"path": cfg.TargetPath,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("marshal target config: %w", err)
	}
	return archive.InitBackend(raw)
}

// transferJobsJSON copies job data from source archive to target backend in JSON format.
func transferJobsJSON(jobs []*schema.Job, src archive.ArchiveBackend, dst archive.ArchiveBackend) error {
	// Transfer cluster configs for all clusters referenced by jobs
	clustersDone := make(map[string]bool)
	for _, job := range jobs {
		if clustersDone[job.Cluster] {
			continue
		}
		clusterCfg, err := src.LoadClusterCfg(job.Cluster)
		if err != nil {
			cclog.Warnf("Retention: load cluster config %q: %v", job.Cluster, err)
		} else {
			if err := dst.StoreClusterCfg(job.Cluster, clusterCfg); err != nil {
				cclog.Warnf("Retention: store cluster config %q: %v", job.Cluster, err)
			}
		}
		clustersDone[job.Cluster] = true
	}

	for _, job := range jobs {
		meta, err := src.LoadJobMeta(job)
		if err != nil {
			cclog.Warnf("Retention: load meta for job %d: %v", job.JobID, err)
			continue
		}
		data, err := src.LoadJobData(job)
		if err != nil {
			cclog.Warnf("Retention: load data for job %d: %v", job.JobID, err)
			continue
		}
		if err := dst.ImportJob(meta, &data); err != nil {
			cclog.Warnf("Retention: import job %d: %v", job.JobID, err)
			continue
		}
	}
	return nil
}

// transferJobsParquet converts jobs to Parquet format, organized by cluster.
func transferJobsParquet(jobs []*schema.Job, src archive.ArchiveBackend, target pqarchive.ParquetTarget, maxSizeMB int) error {
	cw := pqarchive.NewClusterAwareParquetWriter(target, maxSizeMB)

	// Set cluster configs for all clusters referenced by jobs
	clustersDone := make(map[string]bool)
	for _, job := range jobs {
		if clustersDone[job.Cluster] {
			continue
		}
		clusterCfg, err := src.LoadClusterCfg(job.Cluster)
		if err != nil {
			cclog.Warnf("Retention: load cluster config %q: %v", job.Cluster, err)
		} else {
			cw.SetClusterConfig(job.Cluster, clusterCfg)
		}
		clustersDone[job.Cluster] = true
	}

	for _, job := range jobs {
		meta, err := src.LoadJobMeta(job)
		if err != nil {
			cclog.Warnf("Retention: load meta for job %d: %v", job.JobID, err)
			continue
		}
		data, err := src.LoadJobData(job)
		if err != nil {
			cclog.Warnf("Retention: load data for job %d: %v", job.JobID, err)
			continue
		}
		row, err := pqarchive.JobToParquetRow(meta, &data)
		if err != nil {
			cclog.Warnf("Retention: convert job %d: %v", job.JobID, err)
			continue
		}
		if err := cw.AddJob(*row); err != nil {
			cclog.Errorf("Retention: add job %d to writer: %v", job.JobID, err)
			continue
		}
	}

	return cw.Close()
}

// cleanupAfterTransfer removes jobs from archive and optionally from DB.
func cleanupAfterTransfer(jobs []*schema.Job, startTime int64, includeDB bool, omitTagged bool) {
	archive.GetHandle().CleanUp(jobs)

	if includeDB {
		cnt, err := jobRepo.DeleteJobsBefore(startTime, omitTagged)
		if err != nil {
			cclog.Errorf("Retention: delete jobs from db: %v", err)
		} else {
			cclog.Infof("Retention: removed %d jobs from db", cnt)
		}
		if err = jobRepo.Optimize(); err != nil {
			cclog.Errorf("Retention: db optimization error: %v", err)
		}
	}
}

// readCopyMarker reads the last-processed timestamp from a copy marker file.
func readCopyMarker(cfg Retention) int64 {
	var data []byte
	var err error

	switch cfg.TargetKind {
	case "s3":
		// For S3 we store the marker locally alongside the config
		data, err = os.ReadFile(copyMarkerPath(cfg))
	default:
		data, err = os.ReadFile(filepath.Join(cfg.TargetPath, ".copy-marker"))
	}
	if err != nil {
		return 0
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return ts
}

// writeCopyMarker writes the last-processed timestamp to a copy marker file.
func writeCopyMarker(cfg Retention, ts int64) {
	content := []byte(strconv.FormatInt(ts, 10))
	var err error

	switch cfg.TargetKind {
	case "s3":
		err = os.WriteFile(copyMarkerPath(cfg), content, 0o640)
	default:
		err = os.WriteFile(filepath.Join(cfg.TargetPath, ".copy-marker"), content, 0o640)
	}
	if err != nil {
		cclog.Warnf("Retention: write copy marker: %v", err)
	}
}

func copyMarkerPath(cfg Retention) string {
	// For S3 targets, store the marker in a local temp-style path derived from the bucket name
	return filepath.Join(os.TempDir(), fmt.Sprintf("cc-copy-marker-%s", cfg.TargetBucket))
}

func RegisterRetentionDeleteService(cfg Retention) {
	cclog.Info("Register retention delete service")

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(3, 0, 0))),
		gocron.NewTask(
			func() {
				startTime := time.Now().Unix() - int64(cfg.Age*24*3600)
				jobs, err := jobRepo.FindJobsBetween(0, startTime, cfg.OmitTagged)
				if err != nil {
					cclog.Warnf("Retention delete: error finding jobs: %v", err)
					return
				}
				if len(jobs) == 0 {
					return
				}

				cclog.Infof("Retention delete: processing %d jobs", len(jobs))
				cleanupAfterTransfer(jobs, startTime, cfg.IncludeDB, cfg.OmitTagged)
			}))
}

func RegisterRetentionCopyService(cfg Retention) {
	cclog.Infof("Register retention copy service (format=%s, target=%s)", cfg.Format, cfg.TargetKind)

	maxFileSizeMB := cfg.MaxFileSizeMB
	if maxFileSizeMB <= 0 {
		maxFileSizeMB = 512
	}

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(4, 0, 0))),
		gocron.NewTask(
			func() {
				cutoff := time.Now().Unix() - int64(cfg.Age*24*3600)
				lastProcessed := readCopyMarker(cfg)

				jobs, err := jobRepo.FindJobsBetween(lastProcessed, cutoff, cfg.OmitTagged)
				if err != nil {
					cclog.Warnf("Retention copy: error finding jobs: %v", err)
					return
				}
				if len(jobs) == 0 {
					return
				}

				cclog.Infof("Retention copy: processing %d jobs", len(jobs))
				ar := archive.GetHandle()

				switch cfg.Format {
				case "parquet":
					target, err := createParquetTarget(cfg)
					if err != nil {
						cclog.Errorf("Retention copy: create parquet target: %v", err)
						return
					}
					if err := transferJobsParquet(jobs, ar, target, maxFileSizeMB); err != nil {
						cclog.Errorf("Retention copy: parquet transfer: %v", err)
						return
					}
				default: // json
					dst, err := createTargetBackend(cfg)
					if err != nil {
						cclog.Errorf("Retention copy: create target backend: %v", err)
						return
					}
					if err := transferJobsJSON(jobs, ar, dst); err != nil {
						cclog.Errorf("Retention copy: json transfer: %v", err)
						return
					}
				}

				writeCopyMarker(cfg, cutoff)
			}))
}

func RegisterRetentionMoveService(cfg Retention) {
	cclog.Infof("Register retention move service (format=%s, target=%s)", cfg.Format, cfg.TargetKind)

	maxFileSizeMB := cfg.MaxFileSizeMB
	if maxFileSizeMB <= 0 {
		maxFileSizeMB = 512
	}

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(5, 0, 0))),
		gocron.NewTask(
			func() {
				startTime := time.Now().Unix() - int64(cfg.Age*24*3600)
				jobs, err := jobRepo.FindJobsBetween(0, startTime, cfg.OmitTagged)
				if err != nil {
					cclog.Warnf("Retention move: error finding jobs: %v", err)
					return
				}
				if len(jobs) == 0 {
					return
				}

				cclog.Infof("Retention move: processing %d jobs", len(jobs))
				ar := archive.GetHandle()

				switch cfg.Format {
				case "parquet":
					target, err := createParquetTarget(cfg)
					if err != nil {
						cclog.Errorf("Retention move: create parquet target: %v", err)
						return
					}
					if err := transferJobsParquet(jobs, ar, target, maxFileSizeMB); err != nil {
						cclog.Errorf("Retention move: parquet transfer: %v", err)
						return
					}
				default: // json
					dst, err := createTargetBackend(cfg)
					if err != nil {
						cclog.Errorf("Retention move: create target backend: %v", err)
						return
					}
					if err := transferJobsJSON(jobs, ar, dst); err != nil {
						cclog.Errorf("Retention move: json transfer: %v", err)
						return
					}
				}

				cleanupAfterTransfer(jobs, startTime, cfg.IncludeDB, cfg.OmitTagged)
			}))
}
