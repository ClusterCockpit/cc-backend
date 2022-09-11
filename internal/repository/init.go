// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

// `AUTO_INCREMENT` is in a comment because of this hack:
// https://stackoverflow.com/a/41028314 (sqlite creates unique ids automatically)
const JobsDBSchema string = `
	DROP TABLE IF EXISTS jobtag;
	DROP TABLE IF EXISTS job;
	DROP TABLE IF EXISTS tag;

	CREATE TABLE job (
		id                INTEGER PRIMARY KEY /*!40101 AUTO_INCREMENT */,
		job_id            BIGINT NOT NULL,
		cluster           VARCHAR(255) NOT NULL,
		subcluster        VARCHAR(255) NOT NULL,
		start_time        BIGINT NOT NULL, -- Unix timestamp

		user              VARCHAR(255) NOT NULL,
		project           VARCHAR(255) NOT NULL,
		` + "`partition`" + ` VARCHAR(255) NOT NULL, -- partition is a keyword in mysql -.-
		array_job_id      BIGINT NOT NULL,
		duration          INT NOT NULL DEFAULT 0,
		walltime          INT NOT NULL DEFAULT 0,
		job_state         VARCHAR(255) NOT NULL CHECK(job_state IN ('running', 'completed', 'failed', 'cancelled', 'stopped', 'timeout', 'preempted', 'out_of_memory')),
		meta_data         TEXT,          -- JSON
		resources         TEXT NOT NULL, -- JSON

		num_nodes         INT NOT NULL,
		num_hwthreads     INT NOT NULL,
		num_acc           INT NOT NULL,
		smt               TINYINT NOT NULL DEFAULT 1 CHECK(smt               IN (0, 1   )),
		exclusive         TINYINT NOT NULL DEFAULT 1 CHECK(exclusive         IN (0, 1, 2)),
		monitoring_status TINYINT NOT NULL DEFAULT 1 CHECK(monitoring_status IN (0, 1, 2, 3)),

		mem_used_max        REAL NOT NULL DEFAULT 0.0,
		flops_any_avg       REAL NOT NULL DEFAULT 0.0,
		mem_bw_avg          REAL NOT NULL DEFAULT 0.0,
		load_avg            REAL NOT NULL DEFAULT 0.0,
		net_bw_avg          REAL NOT NULL DEFAULT 0.0,
		net_data_vol_total  REAL NOT NULL DEFAULT 0.0,
		file_bw_avg         REAL NOT NULL DEFAULT 0.0,
		file_data_vol_total REAL NOT NULL DEFAULT 0.0);

	CREATE TABLE tag (
		id       INTEGER PRIMARY KEY,
		tag_type VARCHAR(255) NOT NULL,
		tag_name VARCHAR(255) NOT NULL,
		CONSTRAINT be_unique UNIQUE (tag_type, tag_name));

	CREATE TABLE jobtag (
		job_id INTEGER,
		tag_id INTEGER,
		PRIMARY KEY (job_id, tag_id),
		FOREIGN KEY (job_id) REFERENCES job (id) ON DELETE CASCADE,
		FOREIGN KEY (tag_id) REFERENCES tag (id) ON DELETE CASCADE);
`

// Indexes are created after the job-archive is traversed for faster inserts.
const JobsDbIndexes string = `
	CREATE INDEX job_by_user      ON job (user);
	CREATE INDEX job_by_starttime ON job (start_time);
	CREATE INDEX job_by_job_id    ON job (job_id);
	CREATE INDEX job_by_state     ON job (job_state);
`
const NamedJobInsert string = `INSERT INTO job (
	job_id, user, project, cluster, subcluster, ` + "`partition`" + `, array_job_id, num_nodes, num_hwthreads, num_acc,
	exclusive, monitoring_status, smt, job_state, start_time, duration, walltime, resources, meta_data,
	mem_used_max, flops_any_avg, mem_bw_avg, load_avg, net_bw_avg, net_data_vol_total, file_bw_avg, file_data_vol_total
) VALUES (
	:job_id, :user, :project, :cluster, :subcluster, :partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
	:exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :walltime, :resources, :meta_data,
	:mem_used_max, :flops_any_avg, :mem_bw_avg, :load_avg, :net_bw_avg, :net_data_vol_total, :file_bw_avg, :file_data_vol_total
);`

// Delete the tables "job", "tag" and "jobtag" from the database and
// repopulate them using the jobs found in `archive`.
func InitDB() error {
	db := GetConnection()
	starttime := time.Now()
	log.Print("Building job table...")

	// Basic database structure:
	_, err := db.DB.Exec(JobsDBSchema)
	if err != nil {
		return err
	}

	// Inserts are bundled into transactions because in sqlite,
	// that speeds up inserts A LOT.
	tx, err := db.DB.Beginx()
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareNamed(NamedJobInsert)
	if err != nil {
		return err
	}
	tags := make(map[string]int64)

	// Not using log.Print because we want the line to end with `\r` and
	// this function is only ever called when a special command line flag
	// is passed anyways.
	fmt.Printf("%d jobs inserted...\r", 0)

	ar := archive.GetHandle()
	i := 0
	errorOccured := false

	for jobMeta := range ar.Iter() {

		fmt.Printf("Import job %d\n", jobMeta.JobID)
		// // Bundle 100 inserts into one transaction for better performance:
		if i%10 == 0 {
			if tx != nil {
				if err := tx.Commit(); err != nil {
					return err
				}
			}

			tx, err = db.DB.Beginx()
			if err != nil {
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
			log.Errorf("fsBackend LoadClusterCfg()- %v", err)
			errorOccured = true
			continue
		}

		job.RawMetaData, err = json.Marshal(job.MetaData)
		if err != nil {
			log.Errorf("fsBackend LoadClusterCfg()- %v", err)
			errorOccured = true
			continue
		}

		if err := SanityChecks(&job.BaseJob); err != nil {
			log.Errorf("fsBackend LoadClusterCfg()- %v", err)
			errorOccured = true
			continue
		}

		res, err := stmt.Exec(job)
		if err != nil {
			log.Errorf("fsBackend LoadClusterCfg()- %v", err)
			errorOccured = true
			continue
		}

		id, err := res.LastInsertId()
		if err != nil {
			log.Errorf("fsBackend LoadClusterCfg()- %v", err)
			errorOccured = true
			continue
		}

		for _, tag := range job.Tags {
			tagstr := tag.Name + ":" + tag.Type
			tagId, ok := tags[tagstr]
			if !ok {
				res, err := tx.Exec(`INSERT INTO tag (tag_name, tag_type) VALUES (?, ?)`, tag.Name, tag.Type)
				if err != nil {
					return err
				}
				tagId, err = res.LastInsertId()
				if err != nil {
					return err
				}
				tags[tagstr] = tagId
			}

			if _, err := tx.Exec(`INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)`, id, tagId); err != nil {
				return err
			}
		}

		if err == nil {
			i += 1
		}
	}

	if errorOccured {
		log.Errorf("An error occured!")
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Create indexes after inserts so that they do not
	// need to be continually updated.
	if _, err := db.DB.Exec(JobsDbIndexes); err != nil {
		return err
	}

	log.Printf("A total of %d jobs have been registered in %.3f seconds.\n", i, time.Since(starttime).Seconds())
	return nil
}

// This function also sets the subcluster if necessary!
func SanityChecks(job *schema.BaseJob) error {
	if c := archive.GetCluster(job.Cluster); c == nil {
		return fmt.Errorf("no such cluster: %#v", job.Cluster)
	}
	if err := archive.AssignSubCluster(job); err != nil {
		return err
	}
	if !job.State.Valid() {
		return fmt.Errorf("not a valid job state: %#v", job.State)
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
	if job.ArrayJobId == 0 {
		job.ArrayJobId = job.JobID
	}

	return nil
}

func loadJobStat(job *schema.JobMeta, metric string) float64 {
	if stats, ok := job.Statistics[metric]; ok {
		return stats.Avg
	}

	return 0.0
}
