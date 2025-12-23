// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"database/sql"
	"fmt"
	"net/url"
	"sync"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
	"github.com/qustavo/sqlhooks/v2"
)

var (
	dbConnOnce     sync.Once
	dbConnInstance *DBConnection
)

type DBConnection struct {
	DB     *sqlx.DB
	Driver string
}

type DatabaseOptions struct {
	URL                   string
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration
}

func setupSqlite(db *sql.DB) error {
	pragmas := []string{
		"temp_store = memory",
	}

	for _, pragma := range pragmas {
		_, err := db.Exec("PRAGMA " + pragma)
		if err != nil {
			return err
		}
	}

	return nil
}

func Connect(driver string, db string) {
	var err error
	var dbHandle *sqlx.DB

	if driver != "sqlite3" {
		cclog.Abortf("Unsupported database driver '%s'. Only 'sqlite3' is supported.\n", driver)
	}

	dbConnOnce.Do(func() {
		opts := DatabaseOptions{
			URL:                   db,
			MaxOpenConnections:    repoConfig.MaxOpenConnections,
			MaxIdleConnections:    repoConfig.MaxIdleConnections,
			ConnectionMaxLifetime: repoConfig.ConnectionMaxLifetime,
			ConnectionMaxIdleTime: repoConfig.ConnectionMaxIdleTime,
		}

		// TODO: Have separate DB handles for Writes and Reads
		// Optimize SQLite connection: https://kerkour.com/sqlite-for-servers
		connectionURLParams := make(url.Values)
		connectionURLParams.Add("_txlock", "immediate")
		connectionURLParams.Add("_journal_mode", "WAL")
		connectionURLParams.Add("_busy_timeout", "5000")
		connectionURLParams.Add("_synchronous", "NORMAL")
		connectionURLParams.Add("_cache_size", "1000000000")
		connectionURLParams.Add("_foreign_keys", "true")
		opts.URL = fmt.Sprintf("file:%s?%s", opts.URL, connectionURLParams.Encode())

		if cclog.Loglevel() == "debug" {
			sql.Register("sqlite3WithHooks", sqlhooks.Wrap(&sqlite3.SQLiteDriver{}, &Hooks{}))
			dbHandle, err = sqlx.Open("sqlite3WithHooks", opts.URL)
		} else {
			dbHandle, err = sqlx.Open("sqlite3", opts.URL)
		}

		if err != nil {
			cclog.Abortf("DB Connection: Could not connect to SQLite database with sqlx.Open().\nError: %s\n", err.Error())
		}

		err = setupSqlite(dbHandle.DB)
		if err != nil {
			cclog.Abortf("Failed sqlite db setup.\nError: %s\n", err.Error())
		}

		dbHandle.SetMaxOpenConns(opts.MaxOpenConnections)
		dbHandle.SetMaxIdleConns(opts.MaxIdleConnections)
		dbHandle.SetConnMaxLifetime(opts.ConnectionMaxLifetime)
		dbHandle.SetConnMaxIdleTime(opts.ConnectionMaxIdleTime)

		dbConnInstance = &DBConnection{DB: dbHandle, Driver: driver}
		err = checkDBVersion(dbHandle.DB)
		if err != nil {
			cclog.Abortf("DB Connection: Failed DB version check.\nError: %s\n", err.Error())
		}
	})
}

func GetConnection() *DBConnection {
	if dbConnInstance == nil {
		cclog.Fatalf("Database connection not initialized!")
	}

	return dbConnInstance
}

// ResetConnection closes the current database connection and resets the connection state.
// This function is intended for testing purposes only to allow test isolation.
func ResetConnection() error {
	if dbConnInstance != nil && dbConnInstance.DB != nil {
		if err := dbConnInstance.DB.Close(); err != nil {
			return fmt.Errorf("failed to close database connection: %w", err)
		}
	}

	dbConnInstance = nil
	dbConnOnce = sync.Once{}
	jobRepoInstance = nil
	jobRepoOnce = sync.Once{}
	nodeRepoInstance = nil
	nodeRepoOnce = sync.Once{}
	userRepoInstance = nil
	userRepoOnce = sync.Once{}
	userCfgRepoInstance = nil
	userCfgRepoOnce = sync.Once{}

	return nil
}
