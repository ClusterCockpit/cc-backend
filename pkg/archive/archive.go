// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"encoding/json"
	"fmt"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/lrucache"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

const Version uint64 = 1

type ArchiveBackend interface {
	Init(rawConfig json.RawMessage) (uint64, error)

	LoadJobMeta(job *schema.Job) (*schema.JobMeta, error)

	LoadJobData(job *schema.Job) (schema.JobData, error)

	LoadClusterCfg(name string) (*schema.Cluster, error)

	StoreJobMeta(jobMeta *schema.JobMeta) error

	ImportJob(jobMeta *schema.JobMeta, jobData *schema.JobData) error

	GetClusters() []string

	Iter(loadMetricData bool) <-chan JobContainer
}

type JobContainer struct {
	Meta *schema.JobMeta
	Data *schema.JobData
}

var cache *lrucache.Cache = lrucache.New(128 * 1024 * 1024)
var ar ArchiveBackend
var useArchive bool

func Init(rawConfig json.RawMessage, disableArchive bool) error {
	useArchive = !disableArchive
	var kind struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(rawConfig, &kind); err != nil {
		log.Warn("Error while unmarshaling raw config json")
		return err
	}

	switch kind.Kind {
	case "file":
		ar = &FsArchive{}
		// case "s3":
		// 	ar = &S3Archive{}
	default:
		return fmt.Errorf("ARCHIVE/ARCHIVE > unkown archive backend '%s''", kind.Kind)
	}

	version, err := ar.Init(rawConfig)
	if err != nil {
		log.Error("Error while initializing archiveBackend")
		return err
	}
	log.Infof("Load archive version %d", version)
	return initClusterConfig()
}

func GetHandle() ArchiveBackend {
	return ar
}

// Helper to metricdata.LoadAverages().
func LoadAveragesFromArchive(
	job *schema.Job,
	metrics []string,
	data [][]schema.Float) error {

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

func GetStatistics(job *schema.Job) (map[string]schema.JobStatistics, error) {

	metaFile, err := ar.LoadJobMeta(job)
	if err != nil {
		log.Warn("Error while loading job metadata from archiveBackend")
		return nil, err
	}

	return metaFile.Statistics, nil
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
			Name: tag.Name,
			Type: tag.Type,
		})
	}

	return ar.StoreJobMeta(jobMeta)
}
