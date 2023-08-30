// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	_ "github.com/mattn/go-sqlite3"
)

func TestPragma(t *testing.T) {
	t.Run("sets up a new DB", func(t *testing.T) {
		db := setup(t)

		for _, pragma := range []string{"synchronous", "journal_mode", "busy_timeout", "auto_vacuum", "foreign_keys"} {
			t.Log("PRAGMA", pragma, getPragma(db, pragma))
		}
	})
}

func getPragma(db *JobRepository, name string) string {
	var s string
	if err := db.DB.QueryRow(`PRAGMA ` + name).Scan(&s); err != nil {
		panic(err)
	}
	return s
}

func BenchmarkSelect1(b *testing.B) {
	db := setup(b)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := db.DB.Exec(`select 1`)
			noErr(b, err)
		}
	})
}

func BenchmarkDB_FindJobById(b *testing.B) {
	var jobId int64 = 1677322

	b.Run("FindJobById", func(b *testing.B) {
		db := setup(b)

		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := db.FindById(jobId)
				noErr(b, err)
			}
		})
	})
}

func BenchmarkDB_FindJob(b *testing.B) {
	var jobId int64 = 107266
	var startTime int64 = 1657557241
	var cluster = "fritz"

	b.Run("FindJob", func(b *testing.B) {
		db := setup(b)

		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := db.Find(&jobId, &cluster, &startTime)
				noErr(b, err)
			}
		})
	})
}

func BenchmarkDB_CountJobs(b *testing.B) {
	filter := &model.JobFilter{}
	filter.State = append(filter.State, "running")
	cluster := "fritz"
	filter.Cluster = &model.StringInput{Eq: &cluster}
	user := "mppi133h"
	filter.User = &model.StringInput{Eq: &user}

	b.Run("CountJobs", func(b *testing.B) {
		db := setup(b)

		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := db.CountJobs(getContext(b), []*model.JobFilter{filter})
				noErr(b, err)
			}
		})
	})
}

func BenchmarkDB_QueryJobs(b *testing.B) {
	filter := &model.JobFilter{}
	filter.State = append(filter.State, "running")
	cluster := "fritz"
	filter.Cluster = &model.StringInput{Eq: &cluster}
	user := "mppi133h"
	filter.User = &model.StringInput{Eq: &user}
	page := &model.PageRequest{ItemsPerPage: 50, Page: 1}
	order := &model.OrderByInput{Field: "startTime", Order: model.SortDirectionEnumDesc}

	b.Run("QueryJobs", func(b *testing.B) {
		db := setup(b)

		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := db.QueryJobs(getContext(b), []*model.JobFilter{filter}, page, order)
				noErr(b, err)
			}
		})
	})
}

func getContext(tb testing.TB) context.Context {
	tb.Helper()

	var roles []string
	roles = append(roles, schema.GetRoleString(schema.RoleAdmin))
	projects := make([]string, 0)

	user := &schema.User{
		Username:   "demo",
		Name:       "The man",
		Roles:      roles,
		Projects:   projects,
		AuthSource: schema.AuthViaLDAP,
	}
	ctx := context.Background()
	return context.WithValue(ctx, ContextUserKey, user)
}

func setup(tb testing.TB) *JobRepository {
	tb.Helper()
	log.Init("warn", true)
	dbfile := "testdata/job.db"
	err := MigrateDB("sqlite3", dbfile)
	noErr(tb, err)
	Connect("sqlite3", dbfile)
	return GetJobRepository()
}

func noErr(tb testing.TB, err error) {
	tb.Helper()

	if err != nil {
		tb.Fatal("Error is not nil:", err)
	}
}
