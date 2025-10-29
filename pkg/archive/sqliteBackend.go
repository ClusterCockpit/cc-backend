// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archive

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
)

//go:embed migrations/*
var migrationFiles embed.FS

type SqliteArchiveConfig struct {
	Path string `json:"filePath"`
}

type SqliteArchive struct {
	path string
}

func getMigrateInstance(db string) (m *migrate.Migrate, err error) {
	d, err := iofs.New(migrationFiles, "migrations/sqlite3")
	if err != nil {
		cclog.Fatal(err)
	}

	m, err = migrate.NewWithSourceInstance("iofs", d, fmt.Sprintf("sqlite3://%s?_foreign_keys=on", db))
	if err != nil {
		return m, err
	}

	return m, nil
}

func MigrateDB(db string) error {
	m, err := getMigrateInstance(db)
	if err != nil {
		return err
	}

	v, dirty, err := m.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			cclog.Warn("Legacy database without version or missing database file!")
		} else {
			return err
		}
	}

	if uint64(v) < Version {
		cclog.Infof("unsupported database version %d, need %d.\nPlease backup your database file and run cc-backend -migrate-db", v, Version)
	}

	if dirty {
		return fmt.Errorf("last migration to version %d has failed, please fix the db manually and force version with -force-db flag", Version)
	}

	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			cclog.Info("DB already up to date!")
		} else {
			return err
		}
	}

	m.Close()
	return nil
}

func RevertDB(db string) error {
	m, err := getMigrateInstance(db)
	if err != nil {
		return err
	}

	if err := m.Migrate(Version - 1); err != nil {
		if err == migrate.ErrNoChange {
			cclog.Info("DB already up to date!")
		} else {
			return err
		}
	}

	m.Close()
	return nil
}

func ForceDB(db string) error {
	m, err := getMigrateInstance(db)
	if err != nil {
		return err
	}

	if err := m.Force(int(Version)); err != nil {
		return err
	}

	m.Close()
	return nil
}

var (
	dbConnOnce     sync.Once
	dbConnInstance *DBConnection
)

type DBConnection struct {
	DB *sqlx.DB
}

type DatabaseOptions struct {
	URL                   string
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration
}

func setupSqlite(db *sql.DB) (err error) {
	pragmas := []string{
		// "journal_mode = WAL",
		// "busy_timeout = 5000",
		// "synchronous = NORMAL",
		// "cache_size = 1000000000", // 1GB
		// "foreign_keys = true",
		"temp_store = memory",
		// "mmap_size = 3000000000",
	}

	for _, pragma := range pragmas {
		_, err = db.Exec("PRAGMA " + pragma)
		if err != nil {
			return err
		}
	}

	return nil
}

func Connect(driver string, db string) {
	var err error
	var dbHandle *sqlx.DB

	dbConnOnce.Do(func() {
		opts := DatabaseOptions{
			URL:                   db,
			MaxOpenConnections:    4,
			MaxIdleConnections:    4,
			ConnectionMaxLifetime: time.Hour,
			ConnectionMaxIdleTime: time.Hour,
		}

		// TODO: Have separate DB handles for Writes and Reads
		// Optimize SQLite connection: https://kerkour.com/sqlite-for-servers
		connectionUrlParams := make(url.Values)
		connectionUrlParams.Add("_txlock", "immediate")
		connectionUrlParams.Add("_journal_mode", "WAL")
		connectionUrlParams.Add("_busy_timeout", "5000")
		connectionUrlParams.Add("_synchronous", "NORMAL")
		connectionUrlParams.Add("_cache_size", "1000000000")
		connectionUrlParams.Add("_foreign_keys", "true")
		opts.URL = fmt.Sprintf("file:%s?%s", opts.URL, connectionUrlParams.Encode())

		dbHandle, err = sqlx.Open("sqlite3", opts.URL)
		if err != nil {
			cclog.Abortf("Job archive DB Connection: Could not connect to '%s' database with sqlx.Open().\nError: %s\n", driver, err.Error())
		}

		err = setupSqlite(dbHandle.DB)
		if err != nil {
			cclog.Abortf("Job archive DB Connection: Setup Sqlite failed.\nError: %s\n", driver, err.Error())
		}

		dbHandle.SetMaxOpenConns(opts.MaxOpenConnections)
		dbHandle.SetMaxIdleConns(opts.MaxIdleConnections)
		dbHandle.SetConnMaxLifetime(opts.ConnectionMaxLifetime)
		dbHandle.SetConnMaxIdleTime(opts.ConnectionMaxIdleTime)
		dbConnInstance = &DBConnection{DB: dbHandle}

		// err = checkDBVersion(driver, dbHandle.DB)
		// if err != nil {
		// 	cclog.Abortf("DB Connection: Failed DB version check.\nError: %s\n", err.Error())
		// }
	})
}

func GetConnection() *DBConnection {
	if dbConnInstance == nil {
		cclog.Fatalf("Database connection not initialized!")
	}

	return dbConnInstance
}

func (fsa *SqliteArchive) Init(rawConfig json.RawMessage) (uint64, error) {
	return version, nil
}

func (fsa *SqliteArchive) Info() {
	fmt.Printf("SQLITE Job archive\n")
}

func (fsa *SqliteArchive) Exists(job *schema.Job) bool {
}

func (fsa *SqliteArchive) Clean(before int64, after int64) {
}

func (fsa *SqliteArchive) Move(jobs []*schema.Job, path string) {
}

func (fsa *SqliteArchive) CleanUp(jobs []*schema.Job) {
}

func (fsa *SqliteArchive) Compress(jobs []*schema.Job) {
}

func (fsa *SqliteArchive) CompressLast(starttime int64) int64 {
	return 0
}

func (fsa *SqliteArchive) LoadJobData(job *schema.Job) (schema.JobData, error) {
}

func (fsa *SqliteArchive) LoadJobStats(job *schema.Job) (schema.ScopedJobStats, error) {
}

func (fsa *SqliteArchive) LoadJobMeta(job *schema.Job) (*schema.Job, error) {
}

func (fsa *SqliteArchive) LoadClusterCfg(name string) (*schema.Cluster, error) {
}

func (fsa *SqliteArchive) Iter(loadMetricData bool) <-chan JobContainer {
}

func (fsa *SqliteArchive) StoreJobMeta(job *schema.Job) error {
}

func (fsa *SqliteArchive) GetClusters() []string {
}

func (fsa *SqliteArchive) ImportJob(
	jobMeta *schema.Job,
	jobData *schema.JobData,
) error {
}
