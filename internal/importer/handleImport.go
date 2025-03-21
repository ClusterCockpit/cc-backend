// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

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
		job := schema.JobMeta{BaseJob: schema.JobDefaults}
		if err = dec.Decode(&job); err != nil {
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

		job.MonitoringStatus = schema.MonitoringStatusArchivingSuccessful

		sc, err := archive.GetSubCluster(job.Cluster, job.SubCluster)
		if err != nil {
			log.Errorf("cannot get subcluster: %s", err.Error())
			return err
		}

		job.Footprint = make(map[string]float64)

		for _, fp := range sc.Footprint {
			statType := "avg"

			if i, err := archive.MetricIndex(sc.MetricConfig, fp); err != nil {
				statType = sc.MetricConfig[i].Footprint
			}

			name := fmt.Sprintf("%s_%s", fp, statType)

			job.Footprint[name] = repository.LoadJobStat(&job, fp, statType)
		}

		job.RawFootprint, err = json.Marshal(job.Footprint)
		if err != nil {
			log.Warn("Error while marshaling job footprint")
			return err
		}

		job.EnergyFootprint = make(map[string]float64)

		// Total Job Energy Outside Loop
		totalEnergy := 0.0
		for _, fp := range sc.EnergyFootprint {
			// Always Init Metric Energy Inside Loop
			metricEnergy := 0.0
			if i, err := archive.MetricIndex(sc.MetricConfig, fp); err == nil {
				// Note: For DB data, calculate and save as kWh
				if sc.MetricConfig[i].Energy == "energy" { // this metric has energy as unit (Joules)
					log.Warnf("Update EnergyFootprint for Job %d and Metric %s on cluster %s: Set to 'energy' in cluster.json: Not implemented, will return 0.0", job.JobID, job.Cluster, fp)
					// FIXME: Needs sum as stats type
				} else if sc.MetricConfig[i].Energy == "power" { // this metric has power as unit (Watt)
					// Energy: Power (in Watts) * Time (in Seconds)
					// Unit: (W * (s / 3600)) / 1000 = kWh
					// Round 2 Digits: round(Energy * 100) / 100
					// Here: (All-Node Metric Average * Number of Nodes) * (Job Duration in Seconds / 3600) / 1000
					// Note: Shared Jobs handled correctly since "Node Average" is based on partial resources, while "numNodes" factor is 1
					rawEnergy := ((repository.LoadJobStat(&job, fp, "avg") * float64(job.NumNodes)) * (float64(job.Duration) / 3600.0)) / 1000.0
					metricEnergy = math.Round(rawEnergy*100.0) / 100.0
				}
			} else {
				log.Warnf("Error while collecting energy metric %s for job, DB ID '%v', return '0.0'", fp, job.ID)
			}

			job.EnergyFootprint[fp] = metricEnergy
			totalEnergy += metricEnergy
		}

		job.Energy = (math.Round(totalEnergy*100.0) / 100.0)
		if job.RawEnergyFootprint, err = json.Marshal(job.EnergyFootprint); err != nil {
			log.Warnf("Error while marshaling energy footprint for job INTO BYTES, DB ID '%v'", job.ID)
			return err
		}

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

		if err = archive.GetHandle().ImportJob(&job, &jobData); err != nil {
			log.Error("Error while importing job")
			return err
		}

		id, err := r.InsertJob(&job)
		if err != nil {
			log.Warn("Error while job db insert")
			return err
		}

		for _, tag := range job.Tags {
			if err := r.ImportTag(id, tag.Type, tag.Name, tag.Scope); err != nil {
				log.Error("Error while adding or creating tag on import")
				return err
			}
		}

		log.Infof("successfully imported a new job (jobId: %d, cluster: %s, dbid: %d)", job.JobID, job.Cluster, id)
	}
	return nil
}
