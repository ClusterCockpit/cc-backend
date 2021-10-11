package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func initDB(db *sqlx.DB, archive string) error {
	_, err := db.Exec(`
	DROP TABLE IF EXISTS job;
	DROP TABLE IF EXISTS tag;
	DROP TABLE IF EXISTS jobtag;

	CREATE TABLE job (
		id         INTEGER PRIMARY KEY,
		job_id     TEXT,
		user_id    TEXT,
		project_id TEXT,
		cluster_id TEXT,
		start_time TIMESTAMP,
		duration   INTEGER,
		job_state  TEXT,
		num_nodes  INTEGER,
		node_list  TEXT,
		metadata   TEXT,

		flops_any_avg REAL,
		mem_bw_avg    REAL,
		net_bw_avg    REAL,
		file_bw_avg   REAL,
		load_avg      REAL);
	CREATE TABLE tag (
		id       INTEGER PRIMARY KEY,
		tag_type TEXT,
		tag_name TEXT);
	CREATE TABLE jobtag (
		job_id INTEGER,
		tag_id INTEGER,
		PRIMARY KEY (job_id, tag_id),
		FOREIGN KEY (job_id) REFERENCES job (id) ON DELETE CASCADE ON UPDATE NO ACTION,
		FOREIGN KEY (tag_id) REFERENCES tag (id) ON DELETE CASCADE ON UPDATE NO ACTION);

	CREATE INDEX job_by_user ON job (user_id);
	CREATE INDEX job_by_starttime ON job (start_time);`)
	if err != nil {
		return err
	}

	entries0, err := os.ReadDir(archive)
	if err != nil {
		return err
	}

	tags := make(map[string]int64)
	for _, entry0 := range entries0 {
		entries1, err := os.ReadDir(filepath.Join(archive, entry0.Name()))
		if err != nil {
			return err
		}

		for _, entry1 := range entries1 {
			if !entry1.IsDir() {
				continue
			}

			entries2, err := os.ReadDir(filepath.Join(archive, entry0.Name(), entry1.Name()))
			if err != nil {
				return err
			}

			for _, entry2 := range entries2 {
				if err = loadJob(db, tags, filepath.Join(archive, entry0.Name(), entry1.Name(), entry2.Name())); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

type JobMetaFile struct {
	JobId      string   `json:"job_id"`
	UserId     string   `json:"user_id"`
	ProjectId  string   `json:"project_id"`
	ClusterId  string   `json:"cluster_id"`
	NumNodes   int      `json:"num_nodes"`
	JobState   string   `json:"job_state"`
	StartTime  int64    `json:"start_time"`
	Duration   int64    `json:"duration"`
	Nodes      []string `json:"nodes"`
	Tags       []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"tags"`
	Statistics map[string]struct {
		Unit string  `json:"unit"`
		Avg  float64 `json:"avg"`
		Min  float64 `json:"min"`
		Max  float64 `json:"max"`
	} `json:"statistics"`
}

func loadJob(db *sqlx.DB, tags map[string]int64, path string) error {
	f, err := os.Open(filepath.Join(path, "meta.json"))
	if err != nil {
		return err
	}
	defer f.Close()

	var job JobMetaFile
	if err := json.NewDecoder(bufio.NewReader(f)).Decode(&job); err != nil {
		return err
	}

	flopsAnyAvg := loadJobStat(&job, "flops_any")
	memBwAvg := loadJobStat(&job, "mem_bw")
	netBwAvg := loadJobStat(&job, "net_bw")
	fileBwAvg := loadJobStat(&job, "file_bw")
	loadAvg := loadJobStat(&job, "load_one")

	res, err := db.Exec(`
		INSERT INTO job
			(job_id, user_id, project_id, cluster_id, start_time, duration, job_state, num_nodes, node_list, metadata,
			flops_any_avg, mem_bw_avg, net_bw_avg, file_bw_avg, load_avg)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		job.JobId, job.UserId, job.ProjectId, job.ClusterId, job.StartTime, job.Duration, job.JobState, job.NumNodes, strings.Join(job.Nodes, ","), nil,
		flopsAnyAvg, memBwAvg, netBwAvg, fileBwAvg, loadAvg)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}

	if id % 50 == 0 {
		log.Printf("Inserting Job (id: %d, jobId: %s, clusterId: %s)\n", id, job.JobId, job.ClusterId)
	}

	for _, tag := range job.Tags {
		tagstr := tag.Name + ":" + tag.Type
		tagId, ok := tags[tagstr]
		if !ok {
			res, err := db.Exec(`INSERT INTO tag (tag_name, tag_type) VALUES (?, ?)`, tag.Name, tag.Type)
			if err != nil {
				return err
			}
			tagId, err = res.LastInsertId()
			if err != nil {
				return err
			}
			tags[tagstr] = tagId
		}

		if _, err := db.Exec(`INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)`, id, tagId); err != nil {
			return err
		}
	}

	return nil
}

func loadJobStat(job *JobMetaFile, metric string) sql.NullFloat64 {
	val := sql.NullFloat64{Valid: false}
	if stats, ok := job.Statistics[metric]; ok {
		val.Valid = true
		val.Float64 = stats.Avg
	}

	return val
}
