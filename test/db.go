package test

import (
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func InitDB() *sqlx.DB {

	bp := "./"
	ebp := os.Getenv("BASEPATH")

	if ebp != "" {
		bp = ebp + "test/"
	}

	db, err := sqlx.Open("sqlite3", bp+"test.db")
	if err != nil {
		fmt.Println(err)
	}

	return db
}
