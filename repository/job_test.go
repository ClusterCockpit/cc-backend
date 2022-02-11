package repository

import (
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/ClusterCockpit/cc-backend/test"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

var db *sqlx.DB

func init() {
	db = test.InitDB()
}

func setup(t *testing.T) *JobRepository {
	return &JobRepository{
		DB: db,
	}
}

func TestFind(t *testing.T) {
	r := setup(t)

	job, err := r.Find(1404396, "emmy", 1609299584)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Printf("%+v", job)

	if job.ID != 1366 {
		t.Errorf("wrong summary for diagnostic 3\ngot: %d \nwant: 1366", job.JobID)
	}
}

func TestFindById(t *testing.T) {
	r := setup(t)

	job, err := r.FindById(1366)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Printf("%+v", job)

	if job.JobID != 1404396 {
		t.Errorf("wrong summary for diagnostic 3\ngot: %d \nwant: 1404396", job.JobID)
	}
}

func TestGetTags(t *testing.T) {
	r := setup(t)

	tags, counts, err := r.GetTags()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("TAGS %+v \n", tags)
	// fmt.Printf("COUNTS %+v \n", counts)

	if counts["bandwidth"] != 6 {
		t.Errorf("wrong summary for diagnostic 3\ngot: %d \nwant: 6", counts["load-imbalance"])
	}
}
