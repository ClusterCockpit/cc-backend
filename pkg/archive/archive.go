// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/lrucache"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

const Version uint64 = 2

type ArchiveBackend interface {
	Init(rawConfig json.RawMessage) (uint64, error)

	Info()

	Exists(job *schema.Job) bool

	LoadJobMeta(job *schema.Job) (*schema.JobMeta, error)

	LoadJobData(job *schema.Job) (schema.JobData, error)

	LoadJobStats(job *schema.Job) (schema.ScopedJobStats, error)

	LoadClusterCfg(name string) (*schema.Cluster, error)

	StoreJobMeta(jobMeta *schema.JobMeta) error

	ImportJob(jobMeta *schema.JobMeta, jobData *schema.JobData) error

	GetClusters() []string

	CleanUp(jobs []*schema.Job)

	Move(jobs []*schema.Job, path string)

	Clean(before int64, after int64)

	Compress(jobs []*schema.Job)

	CompressLast(starttime int64) int64

	Iter(loadMetricData bool) <-chan JobContainer
}

type JobContainer struct {
	Meta *schema.JobMeta
	Data *schema.JobData
}

var (
	initOnce   sync.Once
	cache      *lrucache.Cache = lrucache.New(128 * 1024 * 1024)
	ar         ArchiveBackend
	useArchive bool
)

func Init(rawConfig json.RawMessage, disableArchive bool) error {
	var err error

	initOnce.Do(func() {
		useArchive = !disableArchive

		var cfg struct {
			Kind string `json:"kind"`
		}

		if err = json.Unmarshal(rawConfig, &cfg); err != nil {
			log.Warn("Error while unmarshaling raw config json")
			return
		}

		switch cfg.Kind {
		case "file":
			ar = &FsArchive{}
			// case "s3":
			// 	ar = &S3Archive{}
		default:
			err = fmt.Errorf("ARCHIVE/ARCHIVE > unkown archive backend '%s''", cfg.Kind)
		}

		var version uint64
		version, err = ar.Init(rawConfig)
		if err != nil {
			log.Error("Error while initializing archiveBackend")
			return
		}
		log.Infof("Load archive version %d", version)

		err = initClusterConfig()
	})

	return err
}

func GetHandle() ArchiveBackend {
	return ar
}

// Helper to metricdataloader.LoadAverages().
func LoadAveragesFromArchive(
	job *schema.Job,
	metrics []string,
	data [][]schema.Float,
) error {
	metaFile, err := ar.LoadJobMeta(job)
	if err != nil {
		log.Warn("Error while loading job metadata from archiveBackend")
		return err
	}

	for i, m := range metrics {
		if stat, ok := metaFile.Statistics[m]; ok {
			data[i] = append(data[i], schema.Float(stat.Avg))
		} else {
			data[i] = append(data[i], schema.NaN)
		}
	}

	return nil
}

// Helper to metricdataloader.LoadJobStats().
func LoadStatsFromArchive(
	job *schema.Job,
	metrics []string,
) (map[string]schema.MetricStatistics, error) {
	data := make(map[string]schema.MetricStatistics, len(metrics))
	metaFile, err := ar.LoadJobMeta(job)
	if err != nil {
		log.Warn("Error while loading job metadata from archiveBackend")
		return data, err
	}

	for _, m := range metrics {
		stat, ok := metaFile.Statistics[m]
		if !ok {
			data[m] = schema.MetricStatistics{Min: 0.0, Avg: 0.0, Max: 0.0}
			continue
		}

		data[m] = schema.MetricStatistics{
			Avg: stat.Avg,
			Min: stat.Min,
			Max: stat.Max,
		}
	}

	return data, nil
}

// Helper to metricdataloader.LoadScopedJobStats().
func LoadScopedStatsFromArchive(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
) (schema.ScopedJobStats, error) {

	data, err := ar.LoadJobStats(job)
	if err != nil {
		log.Warn("Error while loading job metadata from archiveBackend")
		return nil, err
	}

	return data, nil
}

func GetStatistics(job *schema.Job) (map[string]schema.JobStatistics, error) {
	metaFile, err := ar.LoadJobMeta(job)
	if err != nil {
		log.Warn("Error while loading job metadata from archiveBackend")
		return nil, err
	}

	return metaFile.Statistics, nil
}

// If the job is archived, find its `meta.json` file and override the Metadata
// in that JSON file. If the job is not archived, nothing is done.
func UpdateMetadata(job *schema.Job, metadata map[string]string) error {
	if job.State == schema.JobStateRunning || !useArchive {
		return nil
	}

	jobMeta, err := ar.LoadJobMeta(job)
	if err != nil {
		log.Warn("Error while loading job metadata from archiveBackend")
		return err
	}

	for k, v := range metadata {
		jobMeta.MetaData[k] = v
	}

	return ar.StoreJobMeta(jobMeta)
}

// If the job is archived, find its `meta.json` file and override the tags list
// in that JSON file. If the job is not archived, nothing is done.
func UpdateTags(job *schema.Job, tags []*schema.Tag) error {
	if job.State == schema.JobStateRunning || !useArchive {
		return nil
	}

	jobMeta, err := ar.LoadJobMeta(job)
	if err != nil {
		log.Warn("Error while loading job metadata from archiveBackend")
		return err
	}

	jobMeta.Tags = make([]*schema.Tag, 0)
	for _, tag := range tags {
		jobMeta.Tags = append(jobMeta.Tags, &schema.Tag{
			Name:  tag.Name,
			Type:  tag.Type,
			Scope: tag.Scope,
		})
	}

	return ar.StoreJobMeta(jobMeta)
}
