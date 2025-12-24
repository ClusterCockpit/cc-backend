// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// HandleImportFlag imports jobs from file pairs specified in a comma-separated flag string.
//
// The flag format is: "<path-to-meta.json>:<path-to-data.json>[,<path-to-meta2.json>:<path-to-data2.json>,...]"
//
// For each job pair, this function:
//  1. Reads and validates the metadata JSON file (schema.Job)
//  2. Reads and validates the job data JSON file (schema.JobData)
//  3. Enriches the job with calculated footprints and energy metrics
//  4. Validates the job using SanityChecks()
//  5. Imports the job into the archive
//  6. Inserts the job into the database with associated tags
//
// Schema validation is performed if config.Keys.Validate is true.
//
// Returns an error if file reading, validation, enrichment, or database operations fail.
// The function stops processing on the first error encountered.
func HandleImportFlag(flag string) error {
	r := repository.GetJobRepository()

	for _, pair := range strings.Split(flag, ",") {
		files := strings.Split(pair, ":")
		if len(files) != 2 {
			return fmt.Errorf("REPOSITORY/INIT > invalid import flag format")
		}

		raw, err := os.ReadFile(files[0])
		if err != nil {
			cclog.Warn("Error while reading metadata file for import")
			return err
		}

		if config.Keys.Validate {
			if err = schema.Validate(schema.Meta, bytes.NewReader(raw)); err != nil {
				return fmt.Errorf("REPOSITORY/INIT > validate job meta: %v", err)
			}
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		job := schema.Job{
			Shared:           "none",
			MonitoringStatus: schema.MonitoringStatusRunningOrArchiving,
		}
		if err = dec.Decode(&job); err != nil {
			cclog.Warn("Error while decoding raw json metadata for import")
			return err
		}

		raw, err = os.ReadFile(files[1])
		if err != nil {
			cclog.Warn("Error while reading jobdata file for import")
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
			cclog.Warn("Error while decoding raw json jobdata for import")
			return err
		}

		job.MonitoringStatus = schema.MonitoringStatusArchivingSuccessful

		if err = enrichJobMetadata(&job); err != nil {
			cclog.Errorf("Error enriching job metadata: %v", err)
			return err
		}

		if err = SanityChecks(&job); err != nil {
			cclog.Warn("BaseJob SanityChecks failed")
			return err
		}

		if err = archive.GetHandle().ImportJob(&job, &jobData); err != nil {
			cclog.Error("Error while importing job")
			return err
		}

		id, err := r.InsertJob(&job)
		if err != nil {
			cclog.Warn("Error while job db insert")
			return err
		}

		for _, tag := range job.Tags {
			if err := r.ImportTag(id, tag.Type, tag.Name, tag.Scope); err != nil {
				cclog.Error("Error while adding or creating tag on import")
				return err
			}
		}

		cclog.Infof("successfully imported a new job (jobId: %d, cluster: %s, dbid: %d)", job.JobID, job.Cluster, id)
	}
	return nil
}
