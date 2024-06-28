// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

// Import all jobs specified as `<path-to-meta.json>:<path-to-data.json>,...`
func HandleImportFlag(flag string) error {
	r := repository.GetJobRepository()

	for _, pair := range strings.Split(flag, ",") {
		files := strings.Split(pair, ":")
		if len(files) != 2 {
			return fmt.Errorf("REPOSITORY/INIT > invalid import flag format")
		}

		raw, err := os.ReadFile(files[0])
		if err != nil {
			log.Warn("Error while reading metadata file for import")
			return err
		}

		if config.Keys.Validate {
			if err = schema.Validate(schema.Meta, bytes.NewReader(raw)); err != nil {
				return fmt.Errorf("REPOSITORY/INIT > validate job meta: %v", err)
			}
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		jobMeta := schema.JobMeta{BaseJob: schema.JobDefaults}
		if err = dec.Decode(&jobMeta); err != nil {
			log.Warn("Error while decoding raw json metadata for import")
			return err
		}

		raw, err = os.ReadFile(files[1])
		if err != nil {
			log.Warn("Error while reading jobdata file for import")
			return err
		}

		if config.Keys.Validate {
			if err = schema.Validate(schema.Data, bytes.NewReader(raw)); err != nil {
				return fmt.Errorf("REPOSITORY/INIT > validate job data: %v", err)
			}
		}
		dec = json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		jobData := schema.JobData{}
		if err = dec.Decode(&jobData); err != nil {
			log.Warn("Error while decoding raw json jobdata for import")
			return err
		}

		// checkJobData(&jobData)

		jobMeta.MonitoringStatus = schema.MonitoringStatusArchivingSuccessful

		// if _, err = r.Find(&jobMeta.JobID, &jobMeta.Cluster, &jobMeta.StartTime); err != sql.ErrNoRows {
		// 	if err != nil {
		// 		log.Warn("Error while finding job in jobRepository")
		// 		return err
		// 	}
		//
		// 	return fmt.Errorf("REPOSITORY/INIT > a job with that jobId, cluster and startTime does already exist")
		// }
		//
		job := schema.Job{
			BaseJob:       jobMeta.BaseJob,
			StartTime:     time.Unix(jobMeta.StartTime, 0),
			StartTimeUnix: jobMeta.StartTime,
		}

		// TODO: Do loop for new sub structure for stats
		// job.LoadAvg = loadJobStat(&jobMeta, "cpu_load")
		// job.FlopsAnyAvg = loadJobStat(&jobMeta, "flops_any")
		// job.MemUsedMax = loadJobStat(&jobMeta, "mem_used")
		// job.MemBwAvg = loadJobStat(&jobMeta, "mem_bw")
		// job.NetBwAvg = loadJobStat(&jobMeta, "net_bw")
		// job.FileBwAvg = loadJobStat(&jobMeta, "file_bw")

		job.RawResources, err = json.Marshal(job.Resources)
		if err != nil {
			log.Warn("Error while marshaling job resources")
			return err
		}
		job.RawMetaData, err = json.Marshal(job.MetaData)
		if err != nil {
			log.Warn("Error while marshaling job metadata")
			return err
		}

		if err = SanityChecks(&job.BaseJob); err != nil {
			log.Warn("BaseJob SanityChecks failed")
			return err
		}

		if err = archive.GetHandle().ImportJob(&jobMeta, &jobData); err != nil {
			log.Error("Error while importing job")
			return err
		}

		id, err := r.InsertJob(&job)
		if err != nil {
			log.Warn("Error while job db insert")
			return err
		}

		for _, tag := range job.Tags {
			if _, err := r.AddTagOrCreate(id, tag.Type, tag.Name); err != nil {
				log.Error("Error while adding or creating tag")
				return err
			}
		}

		log.Infof("successfully imported a new job (jobId: %d, cluster: %s, dbid: %d)", job.JobID, job.Cluster, id)
	}
	return nil
}
