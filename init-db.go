package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ClusterCockpit/cc-jobarchive/schema"
	"github.com/jmoiron/sqlx"
)

const JOBS_DB_SCHEMA string = `
	DROP TABLE IF EXISTS job;
	DROP TABLE IF EXISTS tag;
	DROP TABLE IF EXISTS jobtag;

	CREATE TABLE job (
		id                INTEGER PRIMARY KEY AUTOINCREMENT, -- Not needed in sqlite
		job_id            BIGINT NOT NULL,
		cluster           VARCHAR(255) NOT NULL,
		start_time        TIMESTAMP NOT NULL,

		user              VARCHAR(255) NOT NULL,
		project           VARCHAR(255) NOT NULL,
		partition         VARCHAR(255) NOT NULL,
		array_job_id      BIGINT NOT NULL,
		duration          INT,
		job_state         VARCHAR(255) CHECK(job_state IN ('running', 'completed', 'failed', 'canceled', 'stopped', 'timeout')) NOT NULL,
		meta_data         TEXT,          -- json, but sqlite has no json type
		resources         TEXT NOT NULL, -- json, but sqlite has no json type

		num_nodes         INT NOT NULL,
		num_hwthreads     INT NOT NULL,
		num_acc           INT NOT NULL,
		smt               TINYINT CHECK(smt IN               (0, 1   )) NOT NULL DEFAULT 1,
		exclusive         TINYINT CHECK(exclusive IN         (0, 1, 2)) NOT NULL DEFAULT 1,
		monitoring_status TINYINT CHECK(monitoring_status IN (0, 1   )) NOT NULL DEFAULT 1,

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
		tag_name VARCHAR(255) NOT NULL);

	CREATE TABLE jobtag (
		job_id INTEGER,
		tag_id INTEGER,
		PRIMARY KEY (job_id, tag_id),
		FOREIGN KEY (job_id) REFERENCES job (id) ON DELETE CASCADE,
		FOREIGN KEY (tag_id) REFERENCES tag (id) ON DELETE CASCADE);
`

// Delete the tables "job", "tag" and "jobtag" from the database and
// repopulate them using the jobs found in `archive`.
func initDB(db *sqlx.DB, archive string) error {
	starttime := time.Now()
	fmt.Println("Building database...")

	// Basic database structure:
	_, err := db.Exec(JOBS_DB_SCHEMA)
	if err != nil {
		return err
	}

	clustersDir, err := os.ReadDir(archive)
	if err != nil {
		return err
	}

	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareNamed(schema.JobInsertStmt)
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

			tx, err = db.Beginx()
			if err != nil {
				return err
			}

			stmt = tx.NamedStmt(stmt)
			fmt.Printf("%d jobs inserted...\r", i)
		}

		err := loadJob(tx, stmt, tags, filename)
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

				for _, startTimeDir := range startTimeDirs {
					if startTimeDir.Type().IsRegular() && startTimeDir.Name() == "meta.json" {
						if err := handleDirectory(dirpath); err != nil {
							log.Printf("in %s: %s\n", dirpath, err.Error())
						}
					} else if startTimeDir.IsDir() {
						if err := handleDirectory(filepath.Join(dirpath, startTimeDir.Name())); err != nil {
							log.Printf("in %s: %s\n", filepath.Join(dirpath, startTimeDir.Name()), err.Error())
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
		CREATE INDEX job_by_user ON job (user);
		CREATE INDEX job_by_starttime ON job (start_time);`); err != nil {
		return err
	}

	log.Printf("A total of %d jobs have been registered in %.3f seconds.\n", i, time.Since(starttime).Seconds())
	return nil
}

// Read the `meta.json` file at `path` and insert it to the database using the prepared
// insert statement `stmt`. `tags` maps all existing tags to their database ID.
func loadJob(tx *sqlx.Tx, stmt *sqlx.NamedStmt, tags map[string]int64, path string) error {
	f, err := os.Open(filepath.Join(path, "meta.json"))
	if err != nil {
		return err
	}
	defer f.Close()

	var job schema.JobMeta = schema.JobMeta{BaseJob: schema.JobDefaults}
	if err := json.NewDecoder(bufio.NewReader(f)).Decode(&job); err != nil {
		return err
	}

	// TODO: Other metrics...
	job.FlopsAnyAvg = loadJobStat(&job, "flops_any")
	job.MemBwAvg = loadJobStat(&job, "mem_bw")

	job.RawResources, err = json.Marshal(job.Resources)
	if err != nil {
		return err
	}

	res, err := stmt.Exec(job)
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

func loadJobStat(job *schema.JobMeta, metric string) float64 {
	if stats, ok := job.Statistics[metric]; ok {
		return stats.Avg
	}

	return 0.0
}
