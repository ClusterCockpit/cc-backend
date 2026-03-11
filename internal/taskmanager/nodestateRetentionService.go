// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package taskmanager

import (
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	pqarchive "github.com/ClusterCockpit/cc-backend/pkg/archive/parquet"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/go-co-op/gocron/v2"
)

func RegisterNodeStateRetentionDeleteService(ageHours int) {
	cclog.Info("Register node state retention delete service")

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(2, 0, 0))),
		gocron.NewTask(
			func() {
				cutoff := time.Now().Unix() - int64(ageHours*3600)
				nodeRepo := repository.GetNodeRepository()
				cnt, err := nodeRepo.DeleteNodeStatesBefore(cutoff)
				if err != nil {
					cclog.Errorf("NodeState retention: error deleting old rows: %v", err)
				} else if cnt > 0 {
					cclog.Infof("NodeState retention: deleted %d old rows", cnt)
				}
			}))
}

func RegisterNodeStateRetentionMoveService(cfg *config.NodeStateRetention) {
	cclog.Info("Register node state retention move service")

	maxFileSizeMB := cfg.MaxFileSizeMB
	if maxFileSizeMB <= 0 {
		maxFileSizeMB = 128
	}

	ageHours := cfg.Age
	if ageHours <= 0 {
		ageHours = 24
	}

	var target pqarchive.ParquetTarget
	var err error

	switch cfg.TargetKind {
	case "s3":
		target, err = pqarchive.NewS3Target(pqarchive.S3TargetConfig{
			Endpoint:     cfg.TargetEndpoint,
			Bucket:       cfg.TargetBucket,
			AccessKey:    cfg.TargetAccessKey,
			SecretKey:    cfg.TargetSecretKey,
			Region:       cfg.TargetRegion,
			UsePathStyle: cfg.TargetUsePathStyle,
		})
	default:
		target, err = pqarchive.NewFileTarget(cfg.TargetPath)
	}

	if err != nil {
		cclog.Errorf("NodeState move retention: failed to create target: %v", err)
		return
	}

	s.NewJob(gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(2, 30, 0))),
		gocron.NewTask(
			func() {
				cutoff := time.Now().Unix() - int64(ageHours*3600)
				nodeRepo := repository.GetNodeRepository()

				rows, err := nodeRepo.FindNodeStatesBefore(cutoff)
				if err != nil {
					cclog.Errorf("NodeState move retention: error finding rows: %v", err)
					return
				}
				if len(rows) == 0 {
					return
				}

				cclog.Infof("NodeState move retention: archiving %d rows", len(rows))
				pw := pqarchive.NewNodeStateParquetWriter(target, maxFileSizeMB)

				for _, ns := range rows {
					row := pqarchive.ParquetNodeStateRow{
						TimeStamp:       ns.TimeStamp,
						NodeState:       ns.NodeState,
						HealthState:     ns.HealthState,
						HealthMetrics:   ns.HealthMetrics,
						CpusAllocated:   int32(ns.CpusAllocated),
						MemoryAllocated: ns.MemoryAllocated,
						GpusAllocated:   int32(ns.GpusAllocated),
						JobsRunning:     int32(ns.JobsRunning),
						Hostname:        ns.Hostname,
						Cluster:         ns.Cluster,
						SubCluster:      ns.SubCluster,
					}
					if err := pw.AddRow(row); err != nil {
						cclog.Errorf("NodeState move retention: add row: %v", err)
						continue
					}
				}

				if err := pw.Close(); err != nil {
					cclog.Errorf("NodeState move retention: close writer: %v", err)
					return
				}

				cnt, err := nodeRepo.DeleteNodeStatesBefore(cutoff)
				if err != nil {
					cclog.Errorf("NodeState move retention: error deleting rows: %v", err)
				} else {
					cclog.Infof("NodeState move retention: deleted %d rows from db", cnt)
				}
			}))
}
