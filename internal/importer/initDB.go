// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package importer provides functionality for importing job data into the ClusterCockpit database.
//
// The package supports two primary use cases:
//  1. Bulk database initialization from archived jobs via InitDB()
//  2. Individual job import from file pairs via HandleImportFlag()
//
// Both operations enrich job metadata by calculating footprints and energy metrics
// before persisting to the database.
package importer

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

const (
	addTagQuery = "INSERT INTO tag (tag_name, tag_type) VALUES (?, ?)"
	setTagQuery = "INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)"
)

// InitDB reinitializes the job database from archived job data.
//
// This function performs the following operations:
//  1. Flushes existing job, tag, and jobtag tables
//  2. Iterates through all jobs in the archive
//  3. Enriches each job with calculated footprints and energy metrics
//  4. Inserts jobs and tags into the database in batched transactions
//
// Jobs are processed in batches of 100 for optimal performance. The function
// continues processing even if individual jobs fail, logging errors and
// returning a summary at the end.
//
// Returns an error if database initialization, transaction management, or
// critical operations fail. Individual job failures are logged but do not
// stop the overall import process.
func InitDB() error {
	r := repository.GetJobRepository()
	if err := r.Flush(); err != nil {
		cclog.Errorf("repository initDB(): %v", err)
		return err
	}
	starttime := time.Now()
	cclog.Print("Building job table...")

	t, err := r.TransactionInit()
	if err != nil {
		cclog.Warn("Error while initializing SQL transactions")
		return err
	}
	tags := make(map[string]int64)

	// Not using cclog.Print because we want the line to end with `\r` and
	// this function is only ever called when a special command line flag
	// is passed anyways.
	fmt.Printf("%d jobs inserted...\r", 0)

	ar := archive.GetHandle()
	i := 0
	errorOccured := 0

	for jobContainer := range ar.Iter(false) {

		jobMeta := jobContainer.Meta
		if jobMeta == nil {
			cclog.Warn("skipping job with nil metadata")
			errorOccured++
			continue
		}

		// Bundle 100 inserts into one transaction for better performance
		if i%100 == 0 {
			if i > 0 {
				if err := t.Commit(); err != nil {
					cclog.Errorf("transaction commit error: %v", err)
					return err
				}
				// Start a new transaction for the next batch
				t, err = r.TransactionInit()
				if err != nil {
					cclog.Errorf("transaction init error: %v", err)
					return err
				}
			}
			fmt.Printf("%d jobs inserted...\r", i)
		}

		jobMeta.MonitoringStatus = schema.MonitoringStatusArchivingSuccessful

		if err := enrichJobMetadata(jobMeta); err != nil {
			cclog.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		if err := SanityChecks(jobMeta); err != nil {
			cclog.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		id, jobErr := r.TransactionAddNamed(t,
			repository.NamedJobInsert, jobMeta)
		if jobErr != nil {
			cclog.Errorf("repository initDB(): %v", jobErr)
			errorOccured++
			continue
		}

		// Job successfully inserted, increment counter
		i += 1

		for _, tag := range jobMeta.Tags {
			tagstr := tag.Name + ":" + tag.Type
			tagID, ok := tags[tagstr]
			if !ok {
				var err error
				tagID, err = r.TransactionAdd(t,
					addTagQuery,
					tag.Name, tag.Type)
				if err != nil {
					cclog.Errorf("Error adding tag: %v", err)
					errorOccured++
					continue
				}
				tags[tagstr] = tagID
			}

			r.TransactionAdd(t,
				setTagQuery,
				id, tagID)
		}
	}

	if errorOccured > 0 {
		cclog.Warnf("Error in import of %d jobs!", errorOccured)
	}

	r.TransactionEnd(t)
	cclog.Infof("A total of %d jobs have been registered in %.3f seconds.", i, time.Since(starttime).Seconds())
	return nil
}

// enrichJobMetadata calculates and populates job footprints, energy metrics, and serialized fields.
//
// This function performs the following enrichment operations:
//  1. Calculates job footprint metrics based on the subcluster configuration
//  2. Computes energy footprint and total energy consumption in kWh
//  3. Marshals footprints, resources, and metadata into JSON for database storage
//
// The function expects the job's MonitoringStatus and SubCluster to be already set.
// Energy calculations convert power metrics (Watts) to energy (kWh) using the formula:
//
//	Energy (kWh) = (Power (W) * Duration (s) / 3600) / 1000
//
// Returns an error if subcluster retrieval, metric indexing, or JSON marshaling fails.
func enrichJobMetadata(job *schema.Job) error {
	sc, err := archive.GetSubCluster(job.Cluster, job.SubCluster)
	if err != nil {
		cclog.Errorf("cannot get subcluster: %s", err.Error())
		return err
	}

	job.Footprint = make(map[string]float64)

	for _, fp := range sc.Footprint {
		statType := "avg"

		if i, err := archive.MetricIndex(sc.MetricConfig, fp); err != nil {
			statType = sc.MetricConfig[i].Footprint
		}

		name := fmt.Sprintf("%s_%s", fp, statType)

		job.Footprint[name] = repository.LoadJobStat(job, fp, statType)
	}

	job.RawFootprint, err = json.Marshal(job.Footprint)
	if err != nil {
		cclog.Warn("Error while marshaling job footprint")
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
			switch sc.MetricConfig[i].Energy {
			case "energy": // this metric has energy as unit (Joules)
				cclog.Warnf("Update EnergyFootprint for Job %d and Metric %s on cluster %s: Set to 'energy' in cluster.json: Not implemented, will return 0.0", job.JobID, job.Cluster, fp)
				// FIXME: Needs sum as stats type
			case "power": // this metric has power as unit (Watt)
				// Energy: Power (in Watts) * Time (in Seconds)
				// Unit: (W * (s / 3600)) / 1000 = kWh
				// Round 2 Digits: round(Energy * 100) / 100
				// Here: (All-Node Metric Average * Number of Nodes) * (Job Duration in Seconds / 3600) / 1000
				// Note: Shared Jobs handled correctly since "Node Average" is based on partial resources, while "numNodes" factor is 1
				rawEnergy := ((repository.LoadJobStat(job, fp, "avg") * float64(job.NumNodes)) * (float64(job.Duration) / 3600.0)) / 1000.0
				metricEnergy = math.Round(rawEnergy*100.0) / 100.0
			}
		} else {
			cclog.Warnf("Error while collecting energy metric %s for job, DB ID '%v', return '0.0'", fp, job.ID)
		}

		job.EnergyFootprint[fp] = metricEnergy
		totalEnergy += metricEnergy
	}

	job.Energy = (math.Round(totalEnergy*100.0) / 100.0)
	if job.RawEnergyFootprint, err = json.Marshal(job.EnergyFootprint); err != nil {
		cclog.Warnf("Error while marshaling energy footprint for job INTO BYTES, DB ID '%v'", job.ID)
		return err
	}

	job.RawResources, err = json.Marshal(job.Resources)
	if err != nil {
		cclog.Warn("Error while marshaling job resources")
		return err
	}

	job.RawMetaData, err = json.Marshal(job.MetaData)
	if err != nil {
		cclog.Warn("Error while marshaling job metadata")
		return err
	}

	return nil
}

// SanityChecks validates job metadata and ensures cluster/subcluster configuration is valid.
//
// This function performs the following validations:
//  1. Verifies the cluster exists in the archive configuration
//  2. Assigns and validates the subcluster (may modify job.SubCluster)
//  3. Validates job state is a recognized value
//  4. Ensures resources and user fields are populated
//  5. Validates node counts and hardware thread counts are positive
//  6. Verifies the number of resources matches the declared node count
//
// The function may modify the job's SubCluster field if it needs to be assigned.
//
// Returns an error if any validation check fails.
func SanityChecks(job *schema.Job) error {
	if c := archive.GetCluster(job.Cluster); c == nil {
		return fmt.Errorf("no such cluster: %v", job.Cluster)
	}
	if err := archive.AssignSubCluster(job); err != nil {
		cclog.Warn("Error while assigning subcluster to job")
		return err
	}
	if !job.State.Valid() {
		return fmt.Errorf("not a valid job state: %v", job.State)
	}
	if len(job.Resources) == 0 || len(job.User) == 0 {
		return fmt.Errorf("'resources' and 'user' should not be empty")
	}
	if job.NumAcc < 0 || job.NumHWThreads < 0 || job.NumNodes < 1 {
		return fmt.Errorf("'numNodes', 'numAcc' or 'numHWThreads' invalid")
	}
	if len(job.Resources) != int(job.NumNodes) {
		return fmt.Errorf("len(resources) does not equal numNodes (%d vs %d)", len(job.Resources), job.NumNodes)
	}

	return nil
}

// checkJobData normalizes metric units in job data based on average values.
//
// NOTE: This function is currently unused and contains incomplete implementation.
// It was intended to normalize byte and file-related metrics to appropriate SI prefixes,
// but the normalization logic is commented out. Consider removing or completing this
// function based on project requirements.
//
// TODO: Either implement the metric normalization or remove this dead code.
func checkJobData(d *schema.JobData) error {
	for _, scopes := range *d {
		// var newUnit schema.Unit
		// TODO Add node scope if missing
		for _, metric := range scopes {
			if strings.Contains(metric.Unit.Base, "B/s") ||
				strings.Contains(metric.Unit.Base, "F/s") ||
				strings.Contains(metric.Unit.Base, "B") {

				// get overall avg
				sum := 0.0
				for _, s := range metric.Series {
					sum += s.Statistics.Avg
				}

				avg := sum / float64(len(metric.Series))
				f, p := Normalize(avg, metric.Unit.Prefix)

				if p != metric.Unit.Prefix {

					fmt.Printf("Convert %e", f)
					// for _, s := range metric.Series {
					// fp := schema.ConvertFloatToFloat64(s.Data)
					//
					// for i := 0; i < len(fp); i++ {
					// 	fp[i] *= f
					// 	fp[i] = math.Ceil(fp[i])
					// }
					//
					// s.Data = schema.GetFloat64ToFloat(fp)
					// }

					metric.Unit.Prefix = p
				}
			}
		}
	}
	return nil
}
