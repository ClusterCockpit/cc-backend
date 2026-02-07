// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	pqarchive "github.com/ClusterCockpit/cc-backend/pkg/archive/parquet"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/go-co-op/gocron/v2"
)

func RegisterRetentionDeleteService(age int, includeDB bool, omitTagged bool) {
	cclog.Info("Register retention delete service")

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(3, 0, 0))),
		gocron.NewTask(
			func() {
				startTime := time.Now().Unix() - int64(age*24*3600)
				jobs, err := jobRepo.FindJobsBetween(0, startTime, omitTagged)
				if err != nil {
					cclog.Warnf("Error while looking for retention jobs: %s", err.Error())
				}
				archive.GetHandle().CleanUp(jobs)

				if includeDB {
					cnt, err := jobRepo.DeleteJobsBefore(startTime, omitTagged)
					if err != nil {
						cclog.Errorf("Error while deleting retention jobs from db: %s", err.Error())
					} else {
						cclog.Infof("Retention: Removed %d jobs from db", cnt)
					}
					if err = jobRepo.Optimize(); err != nil {
						cclog.Errorf("Error occured in db optimization: %s", err.Error())
					}
				}
			}))
}

func RegisterRetentionMoveService(age int, includeDB bool, location string, omitTagged bool) {
	cclog.Info("Register retention move service")

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(4, 0, 0))),
		gocron.NewTask(
			func() {
				startTime := time.Now().Unix() - int64(age*24*3600)
				jobs, err := jobRepo.FindJobsBetween(0, startTime, omitTagged)
				if err != nil {
					cclog.Warnf("Error while looking for retention jobs: %s", err.Error())
				}
				archive.GetHandle().Move(jobs, location)

				if includeDB {
					cnt, err := jobRepo.DeleteJobsBefore(startTime, omitTagged)
					if err != nil {
						cclog.Errorf("Error while deleting retention jobs from db: %v", err)
					} else {
						cclog.Infof("Retention: Removed %d jobs from db", cnt)
					}
					if err = jobRepo.Optimize(); err != nil {
						cclog.Errorf("Error occured in db optimization: %v", err)
					}
				}
			}))
}

func RegisterRetentionParquetService(retention Retention) {
	cclog.Info("Register retention parquet service")

	maxFileSizeMB := retention.MaxFileSizeMB
	if maxFileSizeMB <= 0 {
		maxFileSizeMB = 512
	}

	var target pqarchive.ParquetTarget
	var err error

	switch retention.TargetKind {
	case "s3":
		target, err = pqarchive.NewS3Target(pqarchive.S3TargetConfig{
			Endpoint:     retention.TargetEndpoint,
			Bucket:       retention.TargetBucket,
			AccessKey:    retention.TargetAccessKey,
			SecretKey:    retention.TargetSecretKey,
			Region:       retention.TargetRegion,
			UsePathStyle: retention.TargetUsePathStyle,
		})
	default:
		target, err = pqarchive.NewFileTarget(retention.TargetPath)
	}

	if err != nil {
		cclog.Errorf("Parquet retention: failed to create target: %v", err)
		return
	}

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(5, 0, 0))),
		gocron.NewTask(
			func() {
				startTime := time.Now().Unix() - int64(retention.Age*24*3600)
				jobs, err := jobRepo.FindJobsBetween(0, startTime, retention.OmitTagged)
				if err != nil {
					cclog.Warnf("Parquet retention: error finding jobs: %v", err)
					return
				}
				if len(jobs) == 0 {
					return
				}

				cclog.Infof("Parquet retention: processing %d jobs", len(jobs))
				ar := archive.GetHandle()
				pw := pqarchive.NewParquetWriter(target, maxFileSizeMB)

				for _, job := range jobs {
					meta, err := ar.LoadJobMeta(job)
					if err != nil {
						cclog.Warnf("Parquet retention: load meta for job %d: %v", job.JobID, err)
						continue
					}

					data, err := ar.LoadJobData(job)
					if err != nil {
						cclog.Warnf("Parquet retention: load data for job %d: %v", job.JobID, err)
						continue
					}

					row, err := pqarchive.JobToParquetRow(meta, &data)
					if err != nil {
						cclog.Warnf("Parquet retention: convert job %d: %v", job.JobID, err)
						continue
					}

					if err := pw.AddJob(*row); err != nil {
						cclog.Errorf("Parquet retention: add job %d to writer: %v", job.JobID, err)
						continue
					}
				}

				if err := pw.Close(); err != nil {
					cclog.Errorf("Parquet retention: close writer: %v", err)
					return
				}

				ar.CleanUp(jobs)

				if retention.IncludeDB {
					cnt, err := jobRepo.DeleteJobsBefore(startTime, retention.OmitTagged)
					if err != nil {
						cclog.Errorf("Parquet retention: delete jobs from db: %v", err)
					} else {
						cclog.Infof("Parquet retention: removed %d jobs from db", cnt)
					}
					if err = jobRepo.Optimize(); err != nil {
						cclog.Errorf("Parquet retention: db optimization error: %v", err)
					}
				}
			}))
}
