package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-jobarchive/schema"
	"github.com/jmoiron/sqlx"
)

// Delete the tables "job", "tag" and "jobtag" from the database and
// repopulate them using the jobs found in `archive`.
func initDB(db *sqlx.DB, archive string) error {
	starttime := time.Now()
	fmt.Println("Building database...")

	// Basic database structure:
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
		FOREIGN KEY (tag_id) REFERENCES tag (id) ON DELETE CASCADE ON UPDATE NO ACTION);`)
	if err != nil {
		return err
	}

	clustersDir, err := os.ReadDir(archive)
	if err != nil {
		return err
	}

	insertstmt, err := db.Prepare(`INSERT INTO job
		(job_id, user_id, project_id, cluster_id, start_time, duration, job_state, num_nodes, node_list, metadata, flops_any_avg, mem_bw_avg, net_bw_avg, file_bw_avg, load_avg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	i := 0
	tags := make(map[string]int64)
	handleDirectory := func(filename string) error {
		// Bundle 100 inserts into one transaction for better performance:
		if i%100 == 0 {
			if tx != nil {
				if err := tx.Commit(); err != nil {
					return err
				}
			}

			tx, err = db.Begin()
			if err != nil {
				return err
			}

			insertstmt = tx.Stmt(insertstmt)
			fmt.Printf("%d jobs inserted...\r", i)
		}

		err := loadJob(tx, insertstmt, tags, filename)
		if err == nil {
			i += 1
		}

		return err
	}

	for _, clusterDir := range clustersDir {
		lvl1Dirs, err := os.ReadDir(filepath.Join(archive, clusterDir.Name()))
		if err != nil {
			return err
		}

		for _, lvl1Dir := range lvl1Dirs {
			if !lvl1Dir.IsDir() {
				// Could be the cluster.json file
				continue
			}

			lvl2Dirs, err := os.ReadDir(filepath.Join(archive, clusterDir.Name(), lvl1Dir.Name()))
			if err != nil {
				return err
			}

			for _, lvl2Dir := range lvl2Dirs {
				dirpath := filepath.Join(archive, clusterDir.Name(), lvl1Dir.Name(), lvl2Dir.Name())
				startTimeDirs, err := os.ReadDir(dirpath)
				if err != nil {
					return err
				}

				for _, startTiemDir := range startTimeDirs {
					if startTiemDir.Type().IsRegular() && startTiemDir.Name() == "meta.json" {
						if err := handleDirectory(dirpath); err != nil {
							log.Printf("in %s: %s\n", dirpath, err.Error())
						}
					} else if startTiemDir.IsDir() {
						if err := handleDirectory(filepath.Join(dirpath, startTiemDir.Name())); err != nil {
							log.Printf("in %s: %s\n", filepath.Join(dirpath, startTiemDir.Name()), err.Error())
						}
					}
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Create indexes after inserts so that they do not
	// need to be continually updated.
	if _, err := db.Exec(`
		CREATE INDEX job_by_user ON job (user_id);
		CREATE INDEX job_by_starttime ON job (start_time);`); err != nil {
		return err
	}

	log.Printf("A total of %d jobs have been registered in %.3f seconds.\n", i, time.Since(starttime).Seconds())
	return nil
}

// Read the `meta.json` file at `path` and insert it to the database using the prepared
// insert statement `stmt`. `tags` maps all existing tags to their database ID.
func loadJob(tx *sql.Tx, stmt *sql.Stmt, tags map[string]int64, path string) error {
	f, err := os.Open(filepath.Join(path, "meta.json"))
	if err != nil {
		return err
	}
	defer f.Close()

	var job schema.JobMeta
	if err := json.NewDecoder(bufio.NewReader(f)).Decode(&job); err != nil {
		return err
	}

	flopsAnyAvg := loadJobStat(&job, "flops_any")
	memBwAvg := loadJobStat(&job, "mem_bw")
	netBwAvg := loadJobStat(&job, "net_bw")
	fileBwAvg := loadJobStat(&job, "file_bw")
	loadAvg := loadJobStat(&job, "load_one")

	res, err := stmt.Exec(job.JobId, job.UserId, job.ProjectId, job.ClusterId, job.StartTime, job.Duration, job.JobState,
		job.NumNodes, strings.Join(job.Nodes, ","), nil, flopsAnyAvg, memBwAvg, netBwAvg, fileBwAvg, loadAvg)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
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

	return nil
}

func loadJobStat(job *schema.JobMeta, metric string) sql.NullFloat64 {
	val := sql.NullFloat64{Valid: false}
	if stats, ok := job.Statistics[metric]; ok {
		val.Valid = true
		val.Float64 = stats.Avg
	}

	return val
}
