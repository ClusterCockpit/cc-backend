// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
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
	"strings"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/api"
	"github.com/ClusterCockpit/cc-backend/internal/archiver"
	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph"
	"github.com/ClusterCockpit/cc-backend/internal/metricDataDispatcher"
	"github.com/ClusterCockpit/cc-backend/internal/metricdata"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	ccconf "github.com/ClusterCockpit/cc-lib/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/gorilla/mux"

	_ "github.com/mattn/go-sqlite3"
)

func setup(t *testing.T) *api.RestApi {
	const testconfig = `{
		"main": {
	"addr":            "0.0.0.0:8080",
	"validate": false,
  "apiAllowedIPs": [
    "*"
  ]
	},
	"archive": {
		"kind": "file",
		"path": "./var/job-archive"
	},
	"auth": {
  "jwts": {
      "max-age": "2m"
  }
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

	cclog.Init("info", true)
	tmpdir := t.TempDir()
	jobarchive := filepath.Join(tmpdir, "job-archive")
	if err := os.Mkdir(jobarchive, 0o777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(jobarchive, "version.txt"), fmt.Appendf(nil, "%d", 2), 0o666); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(jobarchive, "testcluster"), 0o777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(jobarchive, "testcluster", "cluster.json"), []byte(testclusterJson), 0o666); err != nil {
		t.Fatal(err)
	}

	dbfilepath := filepath.Join(tmpdir, "test.db")
	err := repository.MigrateDB("sqlite3", dbfilepath)
	if err != nil {
		t.Fatal(err)
	}

	cfgFilePath := filepath.Join(tmpdir, "config.json")
	if err := os.WriteFile(cfgFilePath, []byte(testconfig), 0o666); err != nil {
		t.Fatal(err)
	}

	ccconf.Init(cfgFilePath)

	// Load and check main configuration
	if cfg := ccconf.GetPackageConfig("main"); cfg != nil {
		if clustercfg := ccconf.GetPackageConfig("clusters"); clustercfg != nil {
			config.Init(cfg, clustercfg)
		} else {
			cclog.Abort("Cluster configuration must be present")
		}
	} else {
		cclog.Abort("Main configuration must be present")
	}
	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", jobarchive)

	repository.Connect("sqlite3", dbfilepath)

	if err := archive.Init(json.RawMessage(archiveCfg), config.Keys.DisableArchive); err != nil {
		t.Fatal(err)
	}

	if err := metricdata.Init(); err != nil {
		t.Fatal(err)
	}

	archiver.Start(repository.GetJobRepository())

	if cfg := ccconf.GetPackageConfig("auth"); cfg != nil {
		auth.Init(&cfg)
	} else {
		cclog.Warn("Authentication disabled due to missing configuration")
		auth.Init(nil)
	}

	graph.Init()

	return api.New()
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

	metricdata.TestLoadDataCallback = func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context, resolution int) (schema.JobData, error) {
		return testData, nil
	}

	r := mux.NewRouter()
	r.PathPrefix("/api").Subrouter()
	r.StrictSlash(true)
	restapi.MountApiRoutes(r)

	var TestJobId int64 = 123
	TestClusterName := "testcluster"
	var TestStartTime int64 = 123456789

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
		"shared":           "none",
		"monitoringStatus": 1,
		"smt":              1,
		"resources": [
			{
				"hostname": "host123",
				"hwthreads": [0, 1, 2, 3, 4, 5, 6, 7]
			}
		],
		"metaData":  { "jobScript": "blablabla..." },
		"startTime": 123456789
	}`

	const contextUserKey repository.ContextKey = "user"
	contextUserValue := &schema.User{
		Username:   "testuser",
		Projects:   make([]string, 0),
		Roles:      []string{"user"},
		AuthType:   0,
		AuthSource: 2,
	}

	if ok := t.Run("StartJob", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/jobs/start_job/", bytes.NewBuffer([]byte(startJobBody)))
		recorder := httptest.NewRecorder()

		ctx := context.WithValue(req.Context(), contextUserKey, contextUserValue)

		r.ServeHTTP(recorder, req.WithContext(ctx))
		response := recorder.Result()
		if response.StatusCode != http.StatusCreated {
			t.Fatal(response.Status, recorder.Body.String())
		}
		// resolver := graph.GetResolverInstance()
		restapi.JobRepository.SyncJobs()
		job, err := restapi.JobRepository.Find(&TestJobId, &TestClusterName, &TestStartTime)
		if err != nil {
			t.Fatal(err)
		}

		// job.Tags, err = resolver.Job().Tags(ctx, job)
		// if err != nil {
		// 	t.Fatal(err)
		// }

		if job.JobID != 123 ||
			job.User != "testuser" ||
			job.Project != "testproj" ||
			job.Cluster != "testcluster" ||
			job.SubCluster != "sc1" ||
			job.Partition != "default" ||
			job.Walltime != 3600 ||
			job.ArrayJobID != 0 ||
			job.NumNodes != 1 ||
			job.NumHWThreads != 8 ||
			job.NumAcc != 0 ||
			job.MonitoringStatus != 1 ||
			job.SMT != 1 ||
			!reflect.DeepEqual(job.Resources, []*schema.Resource{{Hostname: "host123", HWThreads: []int{0, 1, 2, 3, 4, 5, 6, 7}}}) ||
			job.StartTime != 123456789 {
			t.Fatalf("unexpected job properties: %#v", job)
		}

		// if len(job.Tags) != 1 || job.Tags[0].Type != "testTagType" || job.Tags[0].Name != "testTagName" || job.Tags[0].Scope != "testuser" {
		// 	t.Fatalf("unexpected tags: %#v", job.Tags)
		// }
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
		req := httptest.NewRequest(http.MethodPost, "/jobs/stop_job/", bytes.NewBuffer([]byte(stopJobBody)))
		recorder := httptest.NewRecorder()

		ctx := context.WithValue(req.Context(), contextUserKey, contextUserValue)

		r.ServeHTTP(recorder, req.WithContext(ctx))
		response := recorder.Result()
		if response.StatusCode != http.StatusOK {
			t.Fatal(response.Status, recorder.Body.String())
		}

		archiver.WaitForArchiving()
		job, err := restapi.JobRepository.Find(&TestJobId, &TestClusterName, &TestStartTime)
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
		data, err := metricDataDispatcher.LoadData(stoppedJob, []string{"load_one"}, []schema.MetricScope{schema.MetricScopeNode}, context.Background(), 60)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(data, testData) {
			t.Fatal("unexpected data fetched from archive")
		}
	})

	t.Run("CheckDoubleStart", func(t *testing.T) {
		// Starting a job with the same jobId and cluster should only be allowed if the startTime is far appart!
		body := strings.ReplaceAll(startJobBody, `"startTime": 123456789`, `"startTime": 123456790`)

		req := httptest.NewRequest(http.MethodPost, "/jobs/start_job/", bytes.NewBuffer([]byte(body)))
		recorder := httptest.NewRecorder()

		ctx := context.WithValue(req.Context(), contextUserKey, contextUserValue)

		r.ServeHTTP(recorder, req.WithContext(ctx))
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
		"shared":        	"none",
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
		req := httptest.NewRequest(http.MethodPost, "/jobs/start_job/", bytes.NewBuffer([]byte(startJobBodyFailed)))
		recorder := httptest.NewRecorder()

		ctx := context.WithValue(req.Context(), contextUserKey, contextUserValue)

		r.ServeHTTP(recorder, req.WithContext(ctx))
		response := recorder.Result()
		if response.StatusCode != http.StatusCreated {
			t.Fatal(response.Status, recorder.Body.String())
		}
	})
	if !ok {
		t.Fatal("subtest failed")
	}

	time.Sleep(1 * time.Second)
	restapi.JobRepository.SyncJobs()

	const stopJobBodyFailed string = `{
    "jobId":     12345,
		"cluster":   "testcluster",

		"jobState": "failed",
		"stopTime": 12355678
	}`

	ok = t.Run("StopJobFailed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/jobs/stop_job/", bytes.NewBuffer([]byte(stopJobBodyFailed)))
		recorder := httptest.NewRecorder()

		ctx := context.WithValue(req.Context(), contextUserKey, contextUserValue)

		r.ServeHTTP(recorder, req.WithContext(ctx))
		response := recorder.Result()
		if response.StatusCode != http.StatusOK {
			t.Fatal(response.Status, recorder.Body.String())
		}

		archiver.WaitForArchiving()
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
