package repository

import (
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

var db *sqlx.DB

func init() {
	var err error

	if db != nil {
		panic("prefer using sub-tests (`t.Run`) or implement `cleanup` before calling setup twice.")
	}
	db, err = sqlx.Open("sqlite3", "../var/test.db")
	if err != nil {
		fmt.Println(err)
	}
}

func setup(t *testing.T) *JobRepository {
	return &JobRepository{
		DB: db,
	}
}

func TestFind(t *testing.T) {
	r := setup(t)

	job, err := r.Find(1001789, "emmy", 1540853248)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Printf("%+v", job)

	if job.ID != 1245 {
		t.Errorf("wrong summary for diagnostic 3\ngot: %d \nwant: 1245", job.JobID)
	}
}

func TestFindById(t *testing.T) {
	r := setup(t)

	job, err := r.FindById(1245)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Printf("%+v", job)

	if job.JobID != 1001789 {
		t.Errorf("wrong summary for diagnostic 3\ngot: %d \nwant: 1001789", job.JobID)
	}
}

func TestGetTags(t *testing.T) {
	r := setup(t)

	tags, _, err := r.GetTags()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("TAGS %+v \n", tags)
	// fmt.Printf("COUNTS %+v \n", counts)
	t.Errorf("wrong summary for diagnostic 3\ngot: %d \nwant: 23", 28)

	// if counts["load-imbalance"] != 23 {
	// 	t.Errorf("wrong summary for diagnostic 3\ngot: %d \nwant: 23", counts["load-imbalance"])
	// }
}
