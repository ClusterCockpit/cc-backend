package test

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
	"clusters": [
	{
	   "name": "testcluster",
	   "metricDataRepository": {"kind": "test", "url": "bla:8081"},
	   "filterRanges": {
		"numNodes": { "from": 1, "to": 64 },
		"duration": { "from": 0, "to": 86400 },
		"startTime": { "from": "2022-01-01T00:00:00Z", "to": null }
	   }
	},
    {
		"name": "taurus",
		"metricDataRepository": {"kind": "test", "url": "bla:8081"},
		 "filterRanges": {
		   "numNodes": { "from": 1, "to": 4000 },
		   "duration": { "from": 0, "to": 604800 },
		   "startTime": { "from": "2010-01-01T00:00:00Z", "to": null }
		 }
	 }
	]
}`
	const testclusterJson = `{
        "name": "testcluster",
		"subClusters": [
			{
				"name": "sc0",
				"nodes": "host120,host121,host122"
			},
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
			    "unit": { "base": "load"},
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

	const taurusclusterJson = `{
		"name": "taurus",
		"SubClusters": [
		  {
			"name": "haswell",
			"processorType": "Intel Haswell",
			"socketsPerNode": 2,
			"coresPerSocket": 12,
			"threadsPerCore": 1, 
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
            "nodes": "w11[27-45,49-63,69-72]",
			"topology": {
			  "node": [ 0, 1 ],
			  "socket": [
				[ 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11 ],
				[ 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23 ]
			  ],
			  "memoryDomain": [
				[ 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11 ],
				[ 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23 ]
			  ],
			  "core": [ [ 0 ], [ 1 ], [ 2 ], [ 3 ], [ 4 ], [ 5 ], [ 6 ], [ 7 ], [ 8 ], [ 9 ], [ 10 ], [ 11 ], [ 12 ], [ 13 ], [ 14 ], [ 15 ], [ 16 ], [ 17 ], [ 18 ], [ 19 ], [ 20 ], [ 21 ], [ 22 ], [ 23 ] ]
			}
		  }
		],
		"metricConfig": [
		  {
			"name": "cpu_used",
			"scope": "core",
			"unit": {"base": ""},
			"aggregation": "avg",
			"timestep": 30,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 1,
				"normal": 0.5,
				"caution": 2e-07,
				"alert": 1e-07
			  }
			]
		  },
		  {
			"name": "ipc",
			"scope": "core",
			"unit": { "base": "IPC"},
            "aggregation": "avg",
			"timestep": 60,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 2,
				"normal": 1,
				"caution": 0.1,
				"alert": 0.5
			  }
			]
		  },
		  {
			"name": "flops_any",
			"scope": "core",
			"unit": { "base": "F/s"},
            "aggregation": "sum",
			"timestep": 60,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 40000000000,
				"normal": 20000000000,
				"caution": 30000000000,
				"alert": 35000000000
			  }
			]
		  },
		  {
			"name": "mem_bw",
			"scope": "socket",
			"unit": { "base": "B/s"},
            "aggregation": "sum",
			"timestep": 60,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 58800000000,
				"normal": 28800000000,
				"caution": 38800000000,
				"alert": 48800000000
			  }
			]
		  },
		  {
			"name": "file_bw",
			"scope": "node",
			"unit": { "base": "B/s"},
            "aggregation": "sum",
			"timestep": 30,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 20000000000,
				"normal": 5000000000,
				"caution": 9000000000,
				"alert": 19000000000
			  }
			]
		  },
		  {
			"name": "net_bw",
			"scope": "node",
			"unit": { "base": "B/s"},
			"timestep": 30,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 7000000000,
				"normal": 5000000000,
				"caution": 6000000000,
				"alert": 6500000000
			  }
			]
		  },
		  {
			"name": "mem_used",
			"scope": "node",
			"unit": {"base": "B"},
            "aggregation": "sum",
			"timestep": 30,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 32000000000,
				"normal": 2000000000,
				"caution": 31000000000,
				"alert": 30000000000
			  }
			]
		  },
		  {
			"name": "cpu_power",
			"scope": "socket",
			"unit": {"base": "W"},
            "aggregation": "sum",
			"timestep": 60,
			"subClusters": [
			  {
				"name": "haswell",
				"peak": 100,
				"normal": 80,
				"caution": 90,
				"alert": 90
			  }
			]
		  }
		]
	}`

	tmpdir := t.TempDir()
	jobarchive := filepath.Join(tmpdir, "job-archive")
	if err := os.Mkdir(jobarchive, 0777); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(jobarchive, "testcluster"), 0777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(jobarchive, "testcluster", "cluster.json"), []byte(testclusterJson), 0666); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(jobarchive, "taurus"), 0777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(jobarchive, "taurus", "cluster.json"), []byte(taurusclusterJson), 0666); err != nil {
		t.Fatal(err)
	}
	dbfilepath := filepath.Join(tmpdir, "test.db")
	f, err := os.Create(dbfilepath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfgFilePath := filepath.Join(tmpdir, "config.json")
	if err := os.WriteFile(cfgFilePath, []byte(testconfig), 0666); err != nil {
		t.Fatal(err)
	}

	config.Init(cfgFilePath)
	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", jobarchive)

	repository.Connect("sqlite3", dbfilepath)
	db := repository.GetConnection()

	if err := archive.Init(json.RawMessage(archiveCfg)); err != nil {
		t.Fatal(err)
	}

	if err := metricdata.Init(config.Keys.DisableArchive); err != nil {
		t.Fatal(err)
	}

	if _, err := db.DB.Exec(repository.JobsDBSchema); err != nil {
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
* at least `setup` modifies global state. Log-Output is redirected to /dev/null on purpose.
 */
func TestRestApi(t *testing.T) {
	restapi := setup(t)
	t.Cleanup(cleanup)

	testData := schema.JobData{
		"load_one": map[schema.MetricScope]*schema.JobMetric{
			schema.MetricScopeNode: {
				Unit:     schema.Unit{Base: "load"},
				Scope:    schema.MetricScopeNode,
				Timestep: 60,
				Series: []schema.Series{
					{
						Hostname:   "host123",
						Statistics: &schema.MetricStatistics{Min: 0.1, Avg: 0.2, Max: 0.3},
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

		restapi.OngoingArchivings.Wait()
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

	t.Run("FailedJob", func(t *testing.T) {
		subtestLetJobFail(t, restapi, r)
	})

	t.Run("ImportJob", func(t *testing.T) {
		testImportFlag(t)
	})
}

func subtestLetJobFail(t *testing.T, restapi *api.RestApi, r *mux.Router) {
	const startJobBody string = `{
"jobId":            12345,
		"user":             "testuser",
		"project":          "testproj",
		"cluster":          "testcluster",
		"partition":        "default",
		"walltime":         3600,
		"arrayJobId":       0,
		"numNodes":         1,
		"numAcc":           0,
		"exclusive":        1,
		"monitoringStatus": 1,
		"smt":              1,
		"tags":             [],
		"resources": [
			{
				"hostname": "host123"
			}
		],
		"metaData":  {},
		"startTime": 12345678
	}`

	ok := t.Run("StartJob", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/start_job/", bytes.NewBuffer([]byte(startJobBody)))
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

	const stopJobBody string = `{
"jobId":     12345,
		"cluster":   "testcluster",

		"jobState": "failed",
		"stopTime": 12355678
	}`

	ok = t.Run("StopJob", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/stop_job/", bytes.NewBuffer([]byte(stopJobBody)))
		recorder := httptest.NewRecorder()

		r.ServeHTTP(recorder, req)
		response := recorder.Result()
		if response.StatusCode != http.StatusOK {
			t.Fatal(response.Status, recorder.Body.String())
		}

		restapi.OngoingArchivings.Wait()
		jobid, cluster := int64(12345), "testcluster"
		job, err := restapi.JobRepository.Find(&jobid, &cluster, nil)
		if err != nil {
			t.Fatal(err)
		}

		if job.State != schema.JobStateCompleted {
			t.Fatal("expected job to be completed")
		}
	})
	if !ok {
		t.Fatal("subtest failed")
	}
}

func testImportFlag(t *testing.T) {
	if err := repository.HandleImportFlag("meta.json:data.json"); err != nil {
		t.Fatal(err)
	}

	repo := repository.GetJobRepository()
	jobId := int64(20639587)
	cluster := "taurus"
	startTime := int64(1635856524)
	job, err := repo.Find(&jobId, &cluster, &startTime)
	if err != nil {
		t.Fatal(err)
	}

	if job.NumNodes != 2 {
		t.Errorf("NumNode: Received %d, expected 2", job.NumNodes)
	}

	ar := archive.GetHandle()
	data, err := ar.LoadJobData(job)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) != 8 {
		t.Errorf("Job data length: Got %d, want 8", len(data))
	}

	r := map[string]string{"mem_used": "GB", "net_bw": "KB/s",
		"cpu_power": "W", "cpu_used": "",
		"file_bw": "KB/s", "flops_any": "F/s",
		"mem_bw": "GB/s", "ipc": "IPC"}

	for name, scopes := range data {
		for _, metric := range scopes {
			if metric.Unit.Base != r[name] {
				t.Errorf("Metric %s unit: Got %s, want %s", name, metric.Unit, r[name])
			}
		}
	}
}
