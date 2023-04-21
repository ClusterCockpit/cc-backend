// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/ClusterCockpit/cc-backend/pkg/units"
)

const NamedJobInsert string = `INSERT INTO job (
	job_id, user, project, cluster, subcluster, ` + "`partition`" + `, array_job_id, num_nodes, num_hwthreads, num_acc,
	exclusive, monitoring_status, smt, job_state, start_time, duration, walltime, resources, meta_data,
	mem_used_max, flops_any_avg, mem_bw_avg, load_avg, net_bw_avg, net_data_vol_total, file_bw_avg, file_data_vol_total
) VALUES (
	:job_id, :user, :project, :cluster, :subcluster, :partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
	:exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :walltime, :resources, :meta_data,
	:mem_used_max, :flops_any_avg, :mem_bw_avg, :load_avg, :net_bw_avg, :net_data_vol_total, :file_bw_avg, :file_data_vol_total
);`

// Import all jobs specified as `<path-to-meta.json>:<path-to-data.json>,...`
func HandleImportFlag(flag string) error {
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
			if err := schema.Validate(schema.Meta, bytes.NewReader(raw)); err != nil {
				return fmt.Errorf("REPOSITORY/INIT > validate job meta: %v", err)
			}
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		jobMeta := schema.JobMeta{BaseJob: schema.JobDefaults}
		if err := dec.Decode(&jobMeta); err != nil {
			log.Warn("Error while decoding raw json metadata for import")
			return err
		}

		raw, err = os.ReadFile(files[1])
		if err != nil {
			log.Warn("Error while reading jobdata file for import")
			return err
		}

		if config.Keys.Validate {
			if err := schema.Validate(schema.Data, bytes.NewReader(raw)); err != nil {
				return fmt.Errorf("REPOSITORY/INIT > validate job data: %v", err)
			}
		}
		dec = json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		jobData := schema.JobData{}
		if err := dec.Decode(&jobData); err != nil {
			log.Warn("Error while decoding raw json jobdata for import")
			return err
		}

		checkJobData(&jobData)
		SanityChecks(&jobMeta.BaseJob)
		jobMeta.MonitoringStatus = schema.MonitoringStatusArchivingSuccessful
		if job, err := GetJobRepository().Find(&jobMeta.JobID, &jobMeta.Cluster, &jobMeta.StartTime); err != sql.ErrNoRows {
			if err != nil {
				log.Warn("Error while finding job in jobRepository")
				return err
			}

			return fmt.Errorf("REPOSITORY/INIT > a job with that jobId, cluster and startTime does already exist (dbid: %d)", job.ID)
		}

		job := schema.Job{
			BaseJob:       jobMeta.BaseJob,
			StartTime:     time.Unix(jobMeta.StartTime, 0),
			StartTimeUnix: jobMeta.StartTime,
		}

		// TODO: Other metrics...
		job.FlopsAnyAvg = loadJobStat(&jobMeta, "flops_any")
		job.MemBwAvg = loadJobStat(&jobMeta, "mem_bw")
		job.NetBwAvg = loadJobStat(&jobMeta, "net_bw")
		job.FileBwAvg = loadJobStat(&jobMeta, "file_bw")
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

		if err := SanityChecks(&job.BaseJob); err != nil {
			log.Warn("BaseJob SanityChecks failed")
			return err
		}

		if err := archive.GetHandle().ImportJob(&jobMeta, &jobData); err != nil {
			log.Error("Error while importing job")
			return err
		}

		res, err := GetConnection().DB.NamedExec(NamedJobInsert, job)
		if err != nil {
			log.Warn("Error while NamedJobInsert")
			return err
		}

		id, err := res.LastInsertId()
		if err != nil {
			log.Warn("Error while getting last insert ID")
			return err
		}

		for _, tag := range job.Tags {
			if _, err := GetJobRepository().AddTagOrCreate(id, tag.Type, tag.Name); err != nil {
				log.Error("Error while adding or creating tag")
				return err
			}
		}

		log.Infof("successfully imported a new job (jobId: %d, cluster: %s, dbid: %d)", job.JobID, job.Cluster, id)
	}
	return nil
}

// Delete the tables "job", "tag" and "jobtag" from the database and
// repopulate them using the jobs found in `archive`.
func InitDB() error {
	db := GetConnection()
	starttime := time.Now()
	log.Print("Building job table...")

	// Inserts are bundled into transactions because in sqlite,
	// that speeds up inserts A LOT.
	tx, err := db.DB.Beginx()
	if err != nil {
		log.Warn("Error while bundling transactions")
		return err
	}

	stmt, err := tx.PrepareNamed(NamedJobInsert)
	if err != nil {
		log.Warn("Error while preparing namedJobInsert")
		return err
	}
	tags := make(map[string]int64)

	// Not using log.Print because we want the line to end with `\r` and
	// this function is only ever called when a special command line flag
	// is passed anyways.
	fmt.Printf("%d jobs inserted...\r", 0)

	ar := archive.GetHandle()
	i := 0
	errorOccured := 0

	for jobContainer := range ar.Iter(false) {

		jobMeta := jobContainer.Meta

		// // Bundle 100 inserts into one transaction for better performance:
		if i%10 == 0 {
			if tx != nil {
				if err := tx.Commit(); err != nil {
					log.Warn("Error while committing transactions for jobMeta")
					return err
				}
			}

			tx, err = db.DB.Beginx()
			if err != nil {
				log.Warn("Error while bundling transactions for jobMeta")
				return err
			}

			stmt = tx.NamedStmt(stmt)
			fmt.Printf("%d jobs inserted...\r", i)
		}

		jobMeta.MonitoringStatus = schema.MonitoringStatusArchivingSuccessful
		job := schema.Job{
			BaseJob:       jobMeta.BaseJob,
			StartTime:     time.Unix(jobMeta.StartTime, 0),
			StartTimeUnix: jobMeta.StartTime,
		}

		// TODO: Other metrics...
		job.FlopsAnyAvg = loadJobStat(jobMeta, "flops_any")
		job.MemBwAvg = loadJobStat(jobMeta, "mem_bw")
		job.NetBwAvg = loadJobStat(jobMeta, "net_bw")
		job.FileBwAvg = loadJobStat(jobMeta, "file_bw")

		job.RawResources, err = json.Marshal(job.Resources)
		if err != nil {
			log.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		job.RawMetaData, err = json.Marshal(job.MetaData)
		if err != nil {
			log.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		if err := SanityChecks(&job.BaseJob); err != nil {
			log.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		res, err := stmt.Exec(job)
		if err != nil {
			log.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		id, err := res.LastInsertId()
		if err != nil {
			log.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		for _, tag := range job.Tags {
			tagstr := tag.Name + ":" + tag.Type
			tagId, ok := tags[tagstr]
			if !ok {
				res, err := tx.Exec(`INSERT INTO tag (tag_name, tag_type) VALUES (?, ?)`, tag.Name, tag.Type)
				if err != nil {
					log.Errorf("Error while inserting tag into tag table: %v (Type %v)", tag.Name, tag.Type)
					return err
				}
				tagId, err = res.LastInsertId()
				if err != nil {
					log.Warn("Error while getting last insert ID")
					return err
				}
				tags[tagstr] = tagId
			}

			if _, err := tx.Exec(`INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)`, id, tagId); err != nil {
				log.Errorf("Error while inserting jobtag into jobtag table: %v (TagID %v)", id, tagId)
				return err
			}
		}

		if err == nil {
			i += 1
		}
	}

	if errorOccured > 0 {
		log.Warnf("Error in import of %d jobs!", errorOccured)
	}

	if err := tx.Commit(); err != nil {
		log.Warn("Error while committing SQL transactions")
		return err
	}

	log.Printf("A total of %d jobs have been registered in %.3f seconds.\n", i, time.Since(starttime).Seconds())
	return nil
}

// This function also sets the subcluster if necessary!
func SanityChecks(job *schema.BaseJob) error {
	if c := archive.GetCluster(job.Cluster); c == nil {
		return fmt.Errorf("no such cluster: %v", job.Cluster)
	}
	if err := archive.AssignSubCluster(job); err != nil {
		log.Warn("Error while assigning subcluster to job")
		return err
	}
	if !job.State.Valid() {
		return fmt.Errorf("not a valid job state: %v", job.State)
	}
	if len(job.Resources) == 0 || len(job.User) == 0 {
		return fmt.Errorf("'resources' and 'user' should not be empty")
	}
	if *job.NumAcc < 0 || *job.NumHWThreads < 0 || job.NumNodes < 1 {
		return fmt.Errorf("'numNodes', 'numAcc' or 'numHWThreads' invalid")
	}
	if len(job.Resources) != int(job.NumNodes) {
		return fmt.Errorf("len(resources) does not equal numNodes (%d vs %d)", len(job.Resources), job.NumNodes)
	}

	return nil
}

func loadJobStat(job *schema.JobMeta, metric string) float64 {
	if stats, ok := job.Statistics[metric]; ok {
		return stats.Avg
	}

	return 0.0
}

func getNormalizationFactor(v float64) (float64, int) {
	count := 0
	scale := -3

	if v > 1000.0 {
		for v > 1000.0 {
			v *= 1e-3
			count++
		}
	} else {
		for v < 1.0 {
			v *= 1e3
			count++
		}
		scale = 3
	}
	return math.Pow10(count * scale), count * scale
}

func normalize(avg float64, u schema.Unit) (float64, schema.Unit) {
	f, e := getNormalizationFactor(avg)

	if e != 0 {

		p := units.NewPrefixFromFactor(units.NewPrefix(*u.Prefix), e)
		np := p.Prefix()
		return f, schema.Unit{Prefix: &np, Base: u.Base}
	}

	return f, u
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
				f, u := normalize(avg, metric.Unit)

				if u.Prefix != metric.Unit.Prefix {

					for _, s := range metric.Series {
						fp := schema.ConvertFloatToFloat64(s.Data)

						for i := 0; i < len(fp); i++ {
							fp[i] *= f
							fp[i] = math.Ceil(fp[i])
						}

						s.Data = schema.GetFloat64ToFloat(fp)
					}

					metric.Unit.Prefix = u.Prefix
				}
			}
		}
	}
	return nil
}
