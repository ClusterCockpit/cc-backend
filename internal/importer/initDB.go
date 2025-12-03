// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package importer

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
)

const (
	addTagQuery = "INSERT INTO tag (tag_name, tag_type) VALUES (?, ?)"
	setTagQuery = "INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)"
)

// Delete the tables "job", "tag" and "jobtag" from the database and
// repopulate them using the jobs found in `archive`.
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

		sc, err := archive.GetSubCluster(jobMeta.Cluster, jobMeta.SubCluster)
		if err != nil {
			cclog.Errorf("cannot get subcluster: %s", err.Error())
			return err
		}

		jobMeta.Footprint = make(map[string]float64)

		for _, fp := range sc.Footprint {
			statType := "avg"

			if i, err := archive.MetricIndex(sc.MetricConfig, fp); err != nil {
				statType = sc.MetricConfig[i].Footprint
			}

			name := fmt.Sprintf("%s_%s", fp, statType)

			jobMeta.Footprint[name] = repository.LoadJobStat(jobMeta, fp, statType)
		}

		jobMeta.RawFootprint, err = json.Marshal(jobMeta.Footprint)
		if err != nil {
			cclog.Warn("Error while marshaling job footprint")
			return err
		}

		jobMeta.EnergyFootprint = make(map[string]float64)

		// Total Job Energy Outside Loop
		totalEnergy := 0.0
		for _, fp := range sc.EnergyFootprint {
			// Always Init Metric Energy Inside Loop
			metricEnergy := 0.0
			if i, err := archive.MetricIndex(sc.MetricConfig, fp); err == nil {
				// Note: For DB data, calculate and save as kWh
				if sc.MetricConfig[i].Energy == "energy" { // this metric has energy as unit (Joules)
					cclog.Warnf("Update EnergyFootprint for Job %d and Metric %s on cluster %s: Set to 'energy' in cluster.json: Not implemented, will return 0.0", jobMeta.JobID, jobMeta.Cluster, fp)
					// FIXME: Needs sum as stats type
				} else if sc.MetricConfig[i].Energy == "power" { // this metric has power as unit (Watt)
					// Energy: Power (in Watts) * Time (in Seconds)
					// Unit: (W * (s / 3600)) / 1000 = kWh
					// Round 2 Digits: round(Energy * 100) / 100
					// Here: (All-Node Metric Average * Number of Nodes) * (Job Duration in Seconds / 3600) / 1000
					// Note: Shared Jobs handled correctly since "Node Average" is based on partial resources, while "numNodes" factor is 1
					rawEnergy := ((repository.LoadJobStat(jobMeta, fp, "avg") * float64(jobMeta.NumNodes)) * (float64(jobMeta.Duration) / 3600.0)) / 1000.0
					metricEnergy = math.Round(rawEnergy*100.0) / 100.0
				}
			} else {
				cclog.Warnf("Error while collecting energy metric %s for job, DB ID '%v', return '0.0'", fp, jobMeta.ID)
			}

			jobMeta.EnergyFootprint[fp] = metricEnergy
			totalEnergy += metricEnergy
		}

		jobMeta.Energy = (math.Round(totalEnergy*100.0) / 100.0)
		if jobMeta.RawEnergyFootprint, err = json.Marshal(jobMeta.EnergyFootprint); err != nil {
			cclog.Warnf("Error while marshaling energy footprint for job INTO BYTES, DB ID '%v'", jobMeta.ID)
			return err
		}

		jobMeta.RawResources, err = json.Marshal(jobMeta.Resources)
		if err != nil {
			cclog.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		jobMeta.RawMetaData, err = json.Marshal(jobMeta.MetaData)
		if err != nil {
			cclog.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		if err := SanityChecks(jobMeta); err != nil {
			cclog.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		id, err := r.TransactionAddNamed(t,
			repository.NamedJobInsert, jobMeta)
		if err != nil {
			cclog.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		for _, tag := range jobMeta.Tags {
			tagstr := tag.Name + ":" + tag.Type
			tagId, ok := tags[tagstr]
			if !ok {
				tagId, err = r.TransactionAdd(t,
					addTagQuery,
					tag.Name, tag.Type)
				if err != nil {
					cclog.Errorf("Error adding tag: %v", err)
					errorOccured++
					continue
				}
				tags[tagstr] = tagId
			}

			r.TransactionAdd(t,
				setTagQuery,
				id, tagId)
		}

		if err == nil {
			i += 1
		}
	}

	if errorOccured > 0 {
		cclog.Warnf("Error in import of %d jobs!", errorOccured)
	}

	r.TransactionEnd(t)
	cclog.Printf("A total of %d jobs have been registered in %.3f seconds.\n", i, time.Since(starttime).Seconds())
	return nil
}

// This function also sets the subcluster if necessary!
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
