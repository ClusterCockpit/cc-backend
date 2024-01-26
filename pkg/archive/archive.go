// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/lrucache"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

const Version uint64 = 1

type ArchiveBackend interface {
	Init(rawConfig json.RawMessage) (uint64, error)

	Info()

	Exists(job *schema.Job) bool

	LoadJobMeta(job *schema.Job) (*schema.JobMeta, error)

	LoadJobData(job *schema.Job) (schema.JobData, error)

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
	cache      *lrucache.Cache = lrucache.New(128 * 1024 * 1024)
	ar         ArchiveBackend
	useArchive bool
)

func getPath(
	job *schema.Job,
	rootPath string,
	file string,
) string {
	return filepath.Join(
		getDirectory(job, rootPath), file)
}

func getDirectory(
	job *schema.Job,
	rootPath string,
) string {
	lvl1, lvl2 := fmt.Sprintf("%d", job.JobID/1000), fmt.Sprintf("%03d", job.JobID%1000)

	return filepath.Join(
		rootPath,
		job.Cluster,
		lvl1, lvl2,
		strconv.FormatInt(job.StartTime.Unix(), 10))
}

func loadJobMeta(b []byte) (*schema.JobMeta, error) {
	if config.Keys.Validate {
		if err := schema.Validate(schema.Meta, bytes.NewReader(b)); err != nil {
			return &schema.JobMeta{}, fmt.Errorf("validate job meta: %v", err)
		}
	}

	return DecodeJobMeta(bytes.NewReader(b))
}

func loadJobData(f io.Reader, key string, isCompressed bool) (schema.JobData, error) {
	if isCompressed {
		r, err := gzip.NewReader(f)
		if err != nil {
			log.Errorf(" %v", err)
			return nil, err
		}
		defer r.Close()

		if config.Keys.Validate {
			if err := schema.Validate(schema.Data, r); err != nil {
				return schema.JobData{}, fmt.Errorf("validate job data: %v", err)
			}
		}

		return DecodeJobData(r, key)
	} else {
		if config.Keys.Validate {
			if err := schema.Validate(schema.Data, bufio.NewReader(f)); err != nil {
				return schema.JobData{}, fmt.Errorf("validate job data: %v", err)
			}
		}
		return DecodeJobData(bufio.NewReader(f), key)
	}
}

func Init(rawConfig json.RawMessage, disableArchive bool) error {
	useArchive = !disableArchive

	var cfg struct {
		Kind string `json:"kind"`
	}

	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		log.Warn("Error while unmarshaling raw config json")
		return err
	}

	switch cfg.Kind {
	case "file":
		ar = &FsArchive{}
		// case "s3":
		// 	ar = &S3Archive{}
	default:
		return fmt.Errorf("ARCHIVE/ARCHIVE > unkown archive backend '%s''", cfg.Kind)
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
