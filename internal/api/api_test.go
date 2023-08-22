// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/api"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/gorilla/mux"

	_ "github.com/mattn/go-sqlite3"
)

func setup(t *testing.T) *api.RestApi {
	const testconfig = `{
	"addr":            "0.0.0.0:8080",
	"validate": false,
	"archive": {
		"kind": "file",
		"path": "./var/job-archive"
	},
    "jwts": {
        "max-age": "2m"
    },
	"clusters": [
	{
	   "name": "testcluster",
	   "metricDataRepository": {"kind": "test", "url": "bla:8081"},
	   "filterRanges": {
		"numNodes": { "from": 1, "to": 64 },
		"duration": { "from": 0, "to": 86400 },
		"startTime": { "from": "2022-01-01T00:00:00Z", "to": null }
	   }
	}
	]
}`
	const testclusterJson = `{
        "name": "testcluster",
		"subClusters": [
			{
				"name": "sc1",
				"nodes": "host123,host124,host125",
				"processorType": "Intel Core i7-4770",
				"socketsPerNode": 1,
				"coresPerSocket": 4,
				"threadsPerCore": 2,
                "flopRateScalar": {
                  "unit": {
                    "prefix": "G",
                    "base": "F/s"
                  },
                  "value": 14
                },
                "flopRateSimd": {
                  "unit": {
                    "prefix": "G",
                    "base": "F/s"
                  },
                  "value": 112
                },
                "memoryBandwidth": {
                  "unit": {
                    "prefix": "G",
                    "base": "B/s"
                  },
                  "value": 24
                },
                "numberOfNodes": 70,
				"topology": {
					"node": [0, 1, 2, 3, 4, 5, 6, 7],
					"socket": [[0, 1, 2, 3, 4, 5, 6, 7]],
					"memoryDomain": [[0, 1, 2, 3, 4, 5, 6, 7]],
					"die": [[0, 1, 2, 3, 4, 5, 6, 7]],
					"core": [[0], [1], [2], [3], [4], [5], [6], [7]]
				}
			}
		],
		"metricConfig": [
			{
				"name": "load_one",
			    "unit": { "base": ""},
				"scope": "node",
				"timestep": 60,
                "aggregation": "avg",
				"peak": 8,
				"normal": 0,
				"caution": 0,
				"alert": 0
			}
		]
	}`

	log.Init("info", true)
	tmpdir := t.TempDir()
	jobarchive := filepath.Join(tmpdir, "job-archive")
	if err := os.Mkdir(jobarchive, 0777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(jobarchive, "version.txt"), []byte(fmt.Sprintf("%d", 1)), 0666); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(jobarchive, "testcluster"), 0777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(jobarchive, "testcluster", "cluster.json"), []byte(testclusterJson), 0666); err != nil {
		t.Fatal(err)
	}

	dbfilepath := filepath.Join(tmpdir, "test.db")
	err := repository.MigrateDB("sqlite3", dbfilepath)
	if err != nil {
		t.Fatal(err)
	}

	cfgFilePath := filepath.Join(tmpdir, "config.json")
	if err := os.WriteFile(cfgFilePath, []byte(testconfig), 0666); err != nil {
		t.Fatal(err)
	}

	config.Init(cfgFilePath)
	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", jobarchive)

	repository.Connect("sqlite3", dbfilepath)
	db := repository.GetConnection()

	if err := archive.Init(json.RawMessage(archiveCfg), config.Keys.DisableArchive); err != nil {
		t.Fatal(err)
	}

	if err := metricdata.Init(config.Keys.DisableArchive); err != nil {
		t.Fatal(err)
	}

	jobRepo := repository.GetJobRepository()
	resolver := &graph.Resolver{DB: db.DB, Repo: jobRepo}

	return &api.RestApi{
		JobRepository: resolver.Repo,
		Resolver:      resolver,
	}
}

func cleanup() {
	// TODO: Clear all caches, reset all modules, etc...
}

/*
* This function starts a job, stops it, and then reads its data from the job-archive.
* Do not run sub-tests in parallel! Tests should not be run in parallel at all, because
* at least `setup` modifies global state.
 */
func TestRestApi(t *testing.T) {
	restapi := setup(t)
	t.Cleanup(cleanup)

	testData := schema.JobData{
		"load_one": map[schema.MetricScope]*schema.JobMetric{
			schema.MetricScopeNode: {
				Unit:     schema.Unit{Base: "load"},
				Timestep: 60,
				Series: []schema.Series{
					{
						Hostname:   "host123",
						Statistics: schema.MetricStatistics{Min: 0.1, Avg: 0.2, Max: 0.3},
						Data:       []schema.Float{0.1, 0.1, 0.1, 0.2, 0.2, 0.2, 0.3, 0.3, 0.3},
					},
				},
			},
		},
	}

	metricdata.TestLoadDataCallback = func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) {
		return testData, nil
	}

	r := mux.NewRouter()
	restapi.MountRoutes(r)

	const startJobBody string = `{
        "jobId":            123,
		"user":             "testuser",
		"project":          "testproj",
		"cluster":          "testcluster",
		"partition":        "default",
		"walltime":         3600,
		"arrayJobId":       0,
		"numNodes":         1,
		"numHwthreads":     8,
		"numAcc":           0,
		"exclusive":        1,
		"monitoringStatus": 1,
		"smt":              1,
		"tags":             [{ "type": "testTagType", "name": "testTagName" }],
		"resources": [
			{
				"hostname": "host123",
				"hwthreads": [0, 1, 2, 3, 4, 5, 6, 7]
			}
		],
		"metaData":  { "jobScript": "blablabla..." },
		"startTime": 123456789
	}`

	var dbid int64
	if ok := t.Run("StartJob", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/start_job/", bytes.NewBuffer([]byte(startJobBody)))
		recorder := httptest.NewRecorder()

		r.ServeHTTP(recorder, req)
		response := recorder.Result()
		if response.StatusCode != http.StatusCreated {
			t.Fatal(response.Status, recorder.Body.String())
		}

		var res api.StartJobApiResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}

		job, err := restapi.Resolver.Query().Job(context.Background(), strconv.Itoa(int(res.DBID)))
		if err != nil {
			t.Fatal(err)
		}

		job.Tags, err = restapi.Resolver.Job().Tags(context.Background(), job)
		if err != nil {
			t.Fatal(err)
		}

		if job.JobID != 123 ||
			job.User != "testuser" ||
			job.Project != "testproj" ||
			job.Cluster != "testcluster" ||
			job.SubCluster != "sc1" ||
			job.Partition != "default" ||
			job.Walltime != 3600 ||
			job.ArrayJobId != 0 ||
			job.NumNodes != 1 ||
			job.NumHWThreads != 8 ||
			job.NumAcc != 0 ||
			job.Exclusive != 1 ||
			job.MonitoringStatus != 1 ||
			job.SMT != 1 ||
			!reflect.DeepEqual(job.Resources, []*schema.Resource{{Hostname: "host123", HWThreads: []int{0, 1, 2, 3, 4, 5, 6, 7}}}) ||
			job.StartTime.Unix() != 123456789 {
			t.Fatalf("unexpected job properties: %#v", job)
		}

		if len(job.Tags) != 1 || job.Tags[0].Type != "testTagType" || job.Tags[0].Name != "testTagName" {
			t.Fatalf("unexpected tags: %#v", job.Tags)
		}

		dbid = res.DBID
	}); !ok {
		return
	}

	const stopJobBody string = `{
        "jobId":     123,
		"startTime": 123456789,
		"cluster":   "testcluster",

		"jobState": "completed",
		"stopTime": 123457789
	}`

	var stoppedJob *schema.Job
	if ok := t.Run("StopJob", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/stop_job/", bytes.NewBuffer([]byte(stopJobBody)))
		recorder := httptest.NewRecorder()

		r.ServeHTTP(recorder, req)
		response := recorder.Result()
		if response.StatusCode != http.StatusOK {
			t.Fatal(response.Status, recorder.Body.String())
		}

		restapi.JobRepository.WaitForArchiving()
		job, err := restapi.Resolver.Query().Job(context.Background(), strconv.Itoa(int(dbid)))
		if err != nil {
			t.Fatal(err)
		}

		if job.State != schema.JobStateCompleted {
			t.Fatal("expected job to be completed")
		}

		if job.Duration != (123457789 - 123456789) {
			t.Fatalf("unexpected job properties: %#v", job)
		}

		job.MetaData, err = restapi.JobRepository.FetchMetadata(job)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(job.MetaData, map[string]string{"jobScript": "blablabla..."}) {
			t.Fatalf("unexpected job.metaData: %#v", job.MetaData)
		}

		stoppedJob = job
	}); !ok {
		return
	}

	t.Run("CheckArchive", func(t *testing.T) {
		data, err := metricdata.LoadData(stoppedJob, []string{"load_one"}, []schema.MetricScope{schema.MetricScopeNode}, context.Background())
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(data, testData) {
			t.Fatal("unexpected data fetched from archive")
		}
	})

	t.Run("CheckDoubleStart", func(t *testing.T) {
		// Starting a job with the same jobId and cluster should only be allowed if the startTime is far appart!
		body := strings.Replace(startJobBody, `"startTime": 123456789`, `"startTime": 123456790`, -1)

		req := httptest.NewRequest(http.MethodPost, "/api/jobs/start_job/", bytes.NewBuffer([]byte(body)))
		recorder := httptest.NewRecorder()

		r.ServeHTTP(recorder, req)
		response := recorder.Result()
		if response.StatusCode != http.StatusUnprocessableEntity {
			t.Fatal(response.Status, recorder.Body.String())
		}
	})

	const startJobBodyFailed string = `{
        "jobId":            12345,
		"user":             "testuser",
		"project":          "testproj",
		"cluster":          "testcluster",
		"partition":        "default",
		"walltime":         3600,
		"numNodes":         1,
		"exclusive":        1,
		"monitoringStatus": 1,
		"smt":              1,
		"resources": [
			{
				"hostname": "host123"
			}
		],
		"startTime": 12345678
	}`

	ok := t.Run("StartJobFailed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/start_job/", bytes.NewBuffer([]byte(startJobBodyFailed)))
		recorder := httptest.NewRecorder()

		r.ServeHTTP(recorder, req)
		response := recorder.Result()
		if response.StatusCode != http.StatusCreated {
			t.Fatal(response.Status, recorder.Body.String())
		}
	})
	if !ok {
		t.Fatal("subtest failed")
	}

	const stopJobBodyFailed string = `{
        "jobId":     12345,
		"cluster":   "testcluster",

		"jobState": "failed",
		"stopTime": 12355678
	}`

	ok = t.Run("StopJobFailed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/stop_job/", bytes.NewBuffer([]byte(stopJobBodyFailed)))
		recorder := httptest.NewRecorder()

		r.ServeHTTP(recorder, req)
		response := recorder.Result()
		if response.StatusCode != http.StatusOK {
			t.Fatal(response.Status, recorder.Body.String())
		}

		restapi.JobRepository.WaitForArchiving()
		jobid, cluster := int64(12345), "testcluster"
		job, err := restapi.JobRepository.Find(&jobid, &cluster, nil)
		if err != nil {
			t.Fatal(err)
		}

		if job.State != schema.JobStateFailed {
			t.Fatal("expected job to be failed")
		}
	})
	if !ok {
		t.Fatal("subtest failed")
	}
}
