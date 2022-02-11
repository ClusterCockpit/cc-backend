package test

import (
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
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
