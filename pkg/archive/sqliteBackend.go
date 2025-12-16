// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archive

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"
	_ "github.com/mattn/go-sqlite3"
)

// SqliteArchiveConfig holds the configuration for the SQLite archive backend.
type SqliteArchiveConfig struct {
	DBPath string `json:"dbPath"` // Path to SQLite database file
}

// SqliteArchive implements ArchiveBackend using a SQLite database with BLOB storage.
// Job metadata and data are stored as JSON BLOBs with indexes for fast queries.
//
// Uses WAL (Write-Ahead Logging) mode for better concurrency and a 64MB cache.
type SqliteArchive struct {
	db       *sql.DB  // SQLite database connection
	clusters []string // List of discovered cluster names
}

// sqliteSchema defines the database schema for SQLite archive backend.
// Jobs table: stores job metadata and data as BLOBs with compression flag
// Clusters table: stores cluster configurations
// Metadata table: stores version and other key-value pairs
const sqliteSchema = `
CREATE TABLE IF NOT EXISTS jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL,
    cluster TEXT NOT NULL,
    start_time INTEGER NOT NULL,
    meta_json BLOB NOT NULL,
    data_json BLOB,
    data_compressed BOOLEAN DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(job_id, cluster, start_time)
);

CREATE INDEX IF NOT EXISTS idx_jobs_cluster ON jobs(cluster);
CREATE INDEX IF NOT EXISTS idx_jobs_start_time ON jobs(start_time);
CREATE INDEX IF NOT EXISTS idx_jobs_lookup ON jobs(cluster, job_id, start_time);

CREATE TABLE IF NOT EXISTS clusters (
    name TEXT PRIMARY KEY,
    config_json BLOB NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

func (sa *SqliteArchive) Init(rawConfig json.RawMessage) (uint64, error) {
	var cfg SqliteArchiveConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		cclog.Warnf("SqliteArchive Init() > Unmarshal error: %#v", err)
		return 0, err
	}

	if cfg.DBPath == "" {
		err := fmt.Errorf("SqliteArchive Init(): empty database path")
		cclog.Errorf("SqliteArchive Init() > config error: %v", err)
		return 0, err
	}

	// Open SQLite database
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		cclog.Errorf("SqliteArchive Init() > failed to open database: %v", err)
		return 0, err
	}
	sa.db = db

	// Set pragmas for better performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-64000", // 64MB cache
		"PRAGMA busy_timeout=5000",
	}
	for _, pragma := range pragmas {
		if _, err := sa.db.Exec(pragma); err != nil {
			cclog.Warnf("SqliteArchive Init() > pragma failed: %v", err)
		}
	}

	// Create schema
	if _, err := sa.db.Exec(sqliteSchema); err != nil {
		cclog.Errorf("SqliteArchive Init() > schema creation failed: %v", err)
		return 0, err
	}

	// Check/set version
	var versionStr string
	err = sa.db.QueryRow("SELECT value FROM metadata WHERE key = 'version'").Scan(&versionStr)
	if err == sql.ErrNoRows {
		// First time initialization, set version
		_, err = sa.db.Exec("INSERT INTO metadata (key, value) VALUES ('version', ?)", fmt.Sprintf("%d", Version))
		if err != nil {
			cclog.Errorf("SqliteArchive Init() > failed to set version: %v", err)
			return 0, err
		}
		versionStr = fmt.Sprintf("%d", Version)
	} else if err != nil {
		cclog.Errorf("SqliteArchive Init() > failed to read version: %v", err)
		return 0, err
	}

	version, err := strconv.ParseUint(versionStr, 10, 64)
	if err != nil {
		cclog.Errorf("SqliteArchive Init() > version parse error: %v", err)
		return 0, err
	}

	if version != Version {
		return version, fmt.Errorf("unsupported version %d, need %d", version, Version)
	}

	// Discover clusters
	sa.clusters = []string{}
	rows, err := sa.db.Query("SELECT DISTINCT cluster FROM jobs ORDER BY cluster")
	if err != nil {
		cclog.Errorf("SqliteArchive Init() > failed to query clusters: %v", err)
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var cluster string
		if err := rows.Scan(&cluster); err != nil {
			cclog.Errorf("SqliteArchive Init() > failed to scan cluster: %v", err)
			continue
		}
		sa.clusters = append(sa.clusters, cluster)
	}

	cclog.Infof("SqliteArchive initialized with database '%s', found %d clusters", cfg.DBPath, len(sa.clusters))
	return version, nil
}

func (sa *SqliteArchive) Info() {
	fmt.Printf("SQLite Job archive database\n")

	ci := make(map[string]*clusterInfo)

	rows, err := sa.db.Query(`
		SELECT cluster, COUNT(*), MIN(start_time), MAX(start_time),
		       SUM(LENGTH(meta_json) + COALESCE(LENGTH(data_json), 0))
		FROM jobs
		GROUP BY cluster
	`)
	if err != nil {
		cclog.Fatalf("SqliteArchive Info() > query failed: %s", err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var cluster string
		var numJobs int
		var dateFirst, dateLast int64
		var diskSize int64

		if err := rows.Scan(&cluster, &numJobs, &dateFirst, &dateLast, &diskSize); err != nil {
			cclog.Errorf("SqliteArchive Info() > scan failed: %v", err)
			continue
		}

		ci[cluster] = &clusterInfo{
			numJobs:   numJobs,
			dateFirst: dateFirst,
			dateLast:  dateLast,
			diskSize:  float64(diskSize) / (1024 * 1024), // Convert to MB
		}
	}

	cit := clusterInfo{dateFirst: time.Now().Unix()}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "cluster\t#jobs\tfrom\tto\tsize (MB)")
	for cluster, clusterInfo := range ci {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%.2f\n", cluster,
			clusterInfo.numJobs,
			time.Unix(clusterInfo.dateFirst, 0),
			time.Unix(clusterInfo.dateLast, 0),
			clusterInfo.diskSize)

		cit.numJobs += clusterInfo.numJobs
		cit.dateFirst = util.Min(cit.dateFirst, clusterInfo.dateFirst)
		cit.dateLast = util.Max(cit.dateLast, clusterInfo.dateLast)
		cit.diskSize += clusterInfo.diskSize
	}

	fmt.Fprintf(w, "TOTAL\t%d\t%s\t%s\t%.2f\n",
		cit.numJobs, time.Unix(cit.dateFirst, 0), time.Unix(cit.dateLast, 0), cit.diskSize)
	w.Flush()
}

func (sa *SqliteArchive) Exists(job *schema.Job) bool {
	var count int
	err := sa.db.QueryRow("SELECT COUNT(*) FROM jobs WHERE job_id = ? AND cluster = ? AND start_time = ?",
		job.JobID, job.Cluster, job.StartTime).Scan(&count)
	return err == nil && count > 0
}

func (sa *SqliteArchive) LoadJobMeta(job *schema.Job) (*schema.Job, error) {
	var metaBlob []byte
	err := sa.db.QueryRow("SELECT meta_json FROM jobs WHERE job_id = ? AND cluster = ? AND start_time = ?",
		job.JobID, job.Cluster, job.StartTime).Scan(&metaBlob)
	if err != nil {
		cclog.Errorf("SqliteArchive LoadJobMeta() > query error: %v", err)
		return nil, err
	}

	if config.Keys.Validate {
		if err := schema.Validate(schema.Meta, bytes.NewReader(metaBlob)); err != nil {
			return nil, fmt.Errorf("validate job meta: %v", err)
		}
	}

	return DecodeJobMeta(bytes.NewReader(metaBlob))
}

func (sa *SqliteArchive) LoadJobData(job *schema.Job) (schema.JobData, error) {
	var dataBlob []byte
	var compressed bool
	err := sa.db.QueryRow("SELECT data_json, data_compressed FROM jobs WHERE job_id = ? AND cluster = ? AND start_time = ?",
		job.JobID, job.Cluster, job.StartTime).Scan(&dataBlob, &compressed)
	if err != nil {
		cclog.Errorf("SqliteArchive LoadJobData() > query error: %v", err)
		return nil, err
	}
	key := fmt.Sprintf("%s:%d:%d", job.Cluster, job.JobID, job.StartTime)

	var reader io.Reader = bytes.NewReader(dataBlob)
	if compressed {
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			cclog.Errorf("SqliteArchive LoadJobData() > gzip error: %v", err)
			return nil, err
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	if config.Keys.Validate {
		data, _ := io.ReadAll(reader)
		if err := schema.Validate(schema.Data, bytes.NewReader(data)); err != nil {
			return schema.JobData{}, fmt.Errorf("validate job data: %v", err)
		}
		return DecodeJobData(bytes.NewReader(data), key)
	}

	return DecodeJobData(reader, key)
}

func (sa *SqliteArchive) LoadJobStats(job *schema.Job) (schema.ScopedJobStats, error) {
	var dataBlob []byte
	var compressed bool
	err := sa.db.QueryRow("SELECT data_json, data_compressed FROM jobs WHERE job_id = ? AND cluster = ? AND start_time = ?",
		job.JobID, job.Cluster, job.StartTime).Scan(&dataBlob, &compressed)
	if err != nil {
		cclog.Errorf("SqliteArchive LoadJobStats() > query error: %v", err)
		return nil, err
	}
	key := fmt.Sprintf("%s:%d:%d", job.Cluster, job.JobID, job.StartTime)

	var reader io.Reader = bytes.NewReader(dataBlob)
	if compressed {
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			cclog.Errorf("SqliteArchive LoadJobStats() > gzip error: %v", err)
			return nil, err
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	if config.Keys.Validate {
		data, _ := io.ReadAll(reader)
		if err := schema.Validate(schema.Data, bytes.NewReader(data)); err != nil {
			return nil, fmt.Errorf("validate job data: %v", err)
		}
		return DecodeJobStats(bytes.NewReader(data), key)
	}

	return DecodeJobStats(reader, key)
}

func (sa *SqliteArchive) LoadClusterCfg(name string) (*schema.Cluster, error) {
	var configBlob []byte
	err := sa.db.QueryRow("SELECT config_json FROM clusters WHERE name = ?", name).Scan(&configBlob)
	if err != nil {
		cclog.Errorf("SqliteArchive LoadClusterCfg() > query error: %v", err)
		return nil, err
	}

	if config.Keys.Validate {
		if err := schema.Validate(schema.ClusterCfg, bytes.NewReader(configBlob)); err != nil {
			cclog.Warnf("Validate cluster config: %v\n", err)
			return &schema.Cluster{}, fmt.Errorf("validate cluster config: %v", err)
		}
	}

	return DecodeCluster(bytes.NewReader(configBlob))
}

func (sa *SqliteArchive) StoreJobMeta(job *schema.Job) error {
	var metaBuf bytes.Buffer
	if err := EncodeJobMeta(&metaBuf, job); err != nil {
		cclog.Error("SqliteArchive StoreJobMeta() > encoding error")
		return err
	}

	now := time.Now().Unix()
	_, err := sa.db.Exec(`
		INSERT INTO jobs (job_id, cluster, start_time, meta_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id, cluster, start_time) DO UPDATE SET
			meta_json = excluded.meta_json,
			updated_at = excluded.updated_at
	`, job.JobID, job.Cluster, job.StartTime, metaBuf.Bytes(), now, now)
	if err != nil {
		cclog.Errorf("SqliteArchive StoreJobMeta() > insert error: %v", err)
		return err
	}

	return nil
}

func (sa *SqliteArchive) ImportJob(jobMeta *schema.Job, jobData *schema.JobData) error {
	var metaBuf, dataBuf bytes.Buffer
	if err := EncodeJobMeta(&metaBuf, jobMeta); err != nil {
		cclog.Error("SqliteArchive ImportJob() > encoding meta error")
		return err
	}
	if err := EncodeJobData(&dataBuf, jobData); err != nil {
		cclog.Error("SqliteArchive ImportJob() > encoding data error")
		return err
	}

	var dataBytes []byte
	var compressed bool

	if dataBuf.Len() > 2000 {
		var compressedBuf bytes.Buffer
		gzipWriter := gzip.NewWriter(&compressedBuf)
		if _, err := gzipWriter.Write(dataBuf.Bytes()); err != nil {
			cclog.Errorf("SqliteArchive ImportJob() > gzip write error: %v", err)
			return err
		}
		if err := gzipWriter.Close(); err != nil {
			cclog.Errorf("SqliteArchive ImportJob() > gzip close error: %v", err)
			return err
		}
		dataBytes = compressedBuf.Bytes()
		compressed = true
	} else {
		dataBytes = dataBuf.Bytes()
		compressed = false
	}

	now := time.Now().Unix()
	_, err := sa.db.Exec(`
		INSERT INTO jobs (job_id, cluster, start_time, meta_json, data_json, data_compressed, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(job_id, cluster, start_time) DO UPDATE SET
			meta_json = excluded.meta_json,
			data_json = excluded.data_json,
			data_compressed = excluded.data_compressed,
			updated_at = excluded.updated_at
	`, jobMeta.JobID, jobMeta.Cluster, jobMeta.StartTime, metaBuf.Bytes(), dataBytes, compressed, now, now)
	if err != nil {
		cclog.Errorf("SqliteArchive ImportJob() > insert error: %v", err)
		return err
	}

	return nil
}

func (sa *SqliteArchive) GetClusters() []string {
	return sa.clusters
}

func (sa *SqliteArchive) CleanUp(jobs []*schema.Job) {
	start := time.Now()
	count := 0

	tx, err := sa.db.Begin()
	if err != nil {
		cclog.Errorf("SqliteArchive CleanUp() > transaction error: %v", err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("DELETE FROM jobs WHERE job_id = ? AND cluster = ? AND start_time = ?")
	if err != nil {
		cclog.Errorf("SqliteArchive CleanUp() > prepare error: %v", err)
		return
	}
	defer stmt.Close()

	for _, job := range jobs {
		if job == nil {
			cclog.Errorf("SqliteArchive CleanUp() error: job is nil")
			continue
		}

		if _, err := stmt.Exec(job.JobID, job.Cluster, job.StartTime); err != nil {
			cclog.Errorf("SqliteArchive CleanUp() > delete error: %v", err)
		} else {
			count++
		}
	}

	if err := tx.Commit(); err != nil {
		cclog.Errorf("SqliteArchive CleanUp() > commit error: %v", err)
		return
	}

	cclog.Infof("Retention Service - Remove %d jobs from SQLite in %s", count, time.Since(start))
}

func (sa *SqliteArchive) Move(jobs []*schema.Job, targetPath string) {
	// For SQLite, "move" means updating the cluster field or similar
	// This is interpretation-dependent; for now we'll just log
	cclog.Warnf("SqliteArchive Move() is not fully implemented - moves within database not applicable")
}

func (sa *SqliteArchive) Clean(before int64, after int64) {
	if after == 0 {
		after = math.MaxInt64
	}

	result, err := sa.db.Exec("DELETE FROM jobs WHERE start_time < ? OR start_time > ?", before, after)
	if err != nil {
		cclog.Fatalf("SqliteArchive Clean() > delete error: %s", err.Error())
	}

	rowsAffected, _ := result.RowsAffected()
	cclog.Infof("SqliteArchive Clean() removed %d jobs", rowsAffected)
}

func (sa *SqliteArchive) Compress(jobs []*schema.Job) {
	var cnt int
	start := time.Now()

	tx, err := sa.db.Begin()
	if err != nil {
		cclog.Errorf("SqliteArchive Compress() > transaction error: %v", err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE jobs SET data_json = ?, data_compressed = 1 WHERE job_id = ? AND cluster = ? AND start_time = ?")
	if err != nil {
		cclog.Errorf("SqliteArchive Compress() > prepare error: %v", err)
		return
	}
	defer stmt.Close()

	for _, job := range jobs {
		var dataBlob []byte
		var compressed bool
		err := sa.db.QueryRow("SELECT data_json, data_compressed FROM jobs WHERE job_id = ? AND cluster = ? AND start_time = ?",
			job.JobID, job.Cluster, job.StartTime).Scan(&dataBlob, &compressed)
		if err != nil || compressed || len(dataBlob) < 2000 {
			continue // Skip if error, already compressed, or too small
		}

		// Compress the data
		var compressedBuf bytes.Buffer
		gzipWriter := gzip.NewWriter(&compressedBuf)
		if _, err := gzipWriter.Write(dataBlob); err != nil {
			cclog.Errorf("SqliteArchive Compress() > gzip error: %v", err)
			gzipWriter.Close()
			continue
		}
		gzipWriter.Close()

		if _, err := stmt.Exec(compressedBuf.Bytes(), job.JobID, job.Cluster, job.StartTime); err != nil {
			cclog.Errorf("SqliteArchive Compress() > update error: %v", err)
		} else {
			cnt++
		}
	}

	if err := tx.Commit(); err != nil {
		cclog.Errorf("SqliteArchive Compress() > commit error: %v", err)
		return
	}

	cclog.Infof("Compression Service - %d jobs in SQLite took %s", cnt, time.Since(start))
}

func (sa *SqliteArchive) CompressLast(starttime int64) int64 {
	var lastStr string
	err := sa.db.QueryRow("SELECT value FROM metadata WHERE key = 'compress_last'").Scan(&lastStr)

	var last int64
	if err == sql.ErrNoRows {
		last = starttime
	} else if err != nil {
		cclog.Errorf("SqliteArchive CompressLast() > query error: %v", err)
		last = starttime
	} else {
		last, err = strconv.ParseInt(lastStr, 10, 64)
		if err != nil {
			cclog.Errorf("SqliteArchive CompressLast() > parse error: %v", err)
			last = starttime
		}
	}

	cclog.Infof("SqliteArchive CompressLast() - start %d last %d", starttime, last)

	// Update timestamp
	_, err = sa.db.Exec(`
		INSERT INTO metadata (key, value) VALUES ('compress_last', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, fmt.Sprintf("%d", starttime))
	if err != nil {
		cclog.Errorf("SqliteArchive CompressLast() > update error: %v", err)
	}

	return last
}

func (sa *SqliteArchive) Iter(loadMetricData bool) <-chan JobContainer {
	ch := make(chan JobContainer)

	go func() {
		defer close(ch)

		rows, err := sa.db.Query("SELECT job_id, cluster, start_time, meta_json, data_json, data_compressed FROM jobs ORDER BY cluster, start_time")
		if err != nil {
			cclog.Fatalf("SqliteArchive Iter() > query error: %s", err.Error())
		}
		defer rows.Close()

		for rows.Next() {
			var jobID int64
			var cluster string
			var startTime int64
			var metaBlob []byte
			var dataBlob []byte
			var compressed bool

			if err := rows.Scan(&jobID, &cluster, &startTime, &metaBlob, &dataBlob, &compressed); err != nil {
				cclog.Errorf("SqliteArchive Iter() > scan error: %v", err)
				continue
			}

			job, err := DecodeJobMeta(bytes.NewReader(metaBlob))
			if err != nil {
				cclog.Errorf("SqliteArchive Iter() > decode meta error: %v", err)
				continue
			}

			if loadMetricData && dataBlob != nil {
				var reader io.Reader = bytes.NewReader(dataBlob)
				if compressed {
					gzipReader, err := gzip.NewReader(reader)
					if err != nil {
						cclog.Errorf("SqliteArchive Iter() > gzip error: %v", err)
						ch <- JobContainer{Meta: job, Data: nil}
						continue
					}
					defer gzipReader.Close()
					reader = gzipReader
				}

				key := fmt.Sprintf("%s:%d:%d", job.Cluster, job.JobID, job.StartTime)
				jobData, err := DecodeJobData(reader, key)
				if err != nil {
					cclog.Errorf("SqliteArchive Iter() > decode data error: %v", err)
					ch <- JobContainer{Meta: job, Data: nil}
				} else {
					ch <- JobContainer{Meta: job, Data: &jobData}
				}
			} else {
				ch <- JobContainer{Meta: job, Data: nil}
			}
		}
	}()

	return ch
}

func (sa *SqliteArchive) StoreClusterCfg(name string, config *schema.Cluster) error {
	var configBuf bytes.Buffer
	if err := EncodeCluster(&configBuf, config); err != nil {
		cclog.Error("SqliteArchive StoreClusterCfg() > encoding error")
		return err
	}

	now := time.Now().Unix()
	_, err := sa.db.Exec(`
		INSERT INTO clusters (name, config_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			config_json = excluded.config_json,
			updated_at = excluded.updated_at
	`, name, configBuf.Bytes(), now)
	if err != nil {
		cclog.Errorf("SqliteArchive StoreClusterCfg() > insert error: %v", err)
		return err
	}

	// Update clusters list if new
	found := slices.Contains(sa.clusters, name)
	if !found {
		sa.clusters = append(sa.clusters, name)
	}

	return nil
}
