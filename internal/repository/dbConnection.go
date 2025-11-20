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

func setupSqlite(db *sql.DB) (err error) {
	pragmas := []string{
		"temp_store = memory",
	}

	for _, pragma := range pragmas {
		_, err = db.Exec("PRAGMA " + pragma)
		if err != nil {
			return
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
			MaxOpenConnections:    repoConfig.MaxOpenConnections,
			MaxIdleConnections:    repoConfig.MaxIdleConnections,
			ConnectionMaxLifetime: repoConfig.ConnectionMaxLifetime,
			ConnectionMaxIdleTime: repoConfig.ConnectionMaxIdleTime,
		}

		switch driver {
		case "sqlite3":
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

			if cclog.Loglevel() == "debug" {
				sql.Register("sqlite3WithHooks", sqlhooks.Wrap(&sqlite3.SQLiteDriver{}, &Hooks{}))
				dbHandle, err = sqlx.Open("sqlite3WithHooks", opts.URL)
			} else {
				dbHandle, err = sqlx.Open("sqlite3", opts.URL)
			}

			setupSqlite(dbHandle.DB)
		case "mysql":
			opts.URL += "?multiStatements=true"
			dbHandle, err = sqlx.Open("mysql", opts.URL)
		default:
			cclog.Abortf("DB Connection: Unsupported database driver '%s'.\n", driver)
		}

		if err != nil {
			cclog.Abortf("DB Connection: Could not connect to '%s' database with sqlx.Open().\nError: %s\n", driver, err.Error())
		}

		dbHandle.SetMaxOpenConns(opts.MaxOpenConnections)
		dbHandle.SetMaxIdleConns(opts.MaxIdleConnections)
		dbHandle.SetConnMaxLifetime(opts.ConnectionMaxLifetime)
		dbHandle.SetConnMaxIdleTime(opts.ConnectionMaxIdleTime)

		dbConnInstance = &DBConnection{DB: dbHandle, Driver: driver}
		err = checkDBVersion(driver, dbHandle.DB)
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
