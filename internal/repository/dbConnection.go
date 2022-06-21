package repository

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	dbConnOnce     sync.Once
	dbConnInstance *DBConnection
)

type DBConnection struct {
	DB *sqlx.DB
}

func Connect(driver string, db string) {
	var err error
	var dbHandle *sqlx.DB

	dbConnOnce.Do(func() {
		if driver == "sqlite3" {
			dbHandle, err = sqlx.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=on", db))
			if err != nil {
				log.Fatal(err)
			}

			// sqlite does not multithread. Having more than one connection open would just mean
			// waiting for locks.
			dbHandle.SetMaxOpenConns(1)
		} else if driver == "mysql" {
			dbHandle, err = sqlx.Open("mysql", fmt.Sprintf("%s?multiStatements=true", db))
			if err != nil {
				log.Fatal(err)
			}

			dbHandle.SetConnMaxLifetime(time.Minute * 3)
			dbHandle.SetMaxOpenConns(10)
			dbHandle.SetMaxIdleConns(10)
		} else {
			log.Fatalf("unsupported database driver: %s", driver)
		}

		dbConnInstance = &DBConnection{DB: dbHandle}
	})
}

func GetConnection() *DBConnection {
	if dbConnInstance == nil {
		log.Fatalf("Database connection not initialized!")
	}

	return dbConnInstance
}
