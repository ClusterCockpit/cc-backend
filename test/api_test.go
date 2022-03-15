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
	"testing"

	"github.com/ClusterCockpit/cc-backend/api"
	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/graph"
	"github.com/ClusterCockpit/cc-backend/metricdata"
	"github.com/ClusterCockpit/cc-backend/repository"
	"github.com/ClusterCockpit/cc-backend/schema"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"

	_ "github.com/mattn/go-sqlite3"
)

func setup(t *testing.T) *api.RestApi {
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
				"flopRateScalar": 44,
				"flopRateSimd": 704,
				"memoryBandwidth": 80,
				"topology": {
					"node": [0, 1, 2, 3, 4, 5, 6, 7],
					"socket": [[0, 1, 2, 3, 4, 5, 6, 7]],
					"memoryDomain": [[0, 1, 2, 3, 4, 5, 6, 7]],
					"die": [[0, 1, 2, 3, 4, 5, 6, 7]],
					"core": [[0], [1], [2], [3], [4], [5], [6], [7]],
					"accelerators": []
				}
			}
		],
		"metricDataRepository": {"kind": "test"},
		"metricConfig": [
			{
				"name": "load_one",
				"unit": "load",
				"scope": "node",
				"timestep": 60,
				"peak": 8,
				"normal": 0,
				"caution": 0,
				"alert": 0
			}
		],
		"filterRanges": {
			"numNodes": { "from": 1, "to": 1 },
			"duration": { "from": 0, "to": 172800 },
			"startTime": { "from": "2010-01-01T00:00:00Z", "to": null }
		}
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

	dbfilepath := filepath.Join(tmpdir, "test.db")
	f, err := os.Create(dbfilepath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	db, err := sqlx.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=on", dbfilepath))
	if err != nil {
		t.Fatal(err)
	}

	db.SetMaxOpenConns(1)
	if _, err := db.Exec(repository.JobsDBSchema); err != nil {
		t.Fatal(err)
	}

	if err := config.Init(db, false, map[string]interface{}{}, jobarchive); err != nil {
		t.Fatal(err)
	}

	if err := metricdata.Init(jobarchive, false); err != nil {
		t.Fatal(err)
	}

	resolver := &graph.Resolver{DB: db, Repo: &repository.JobRepository{DB: db}}
	if err := resolver.Repo.Init(); err != nil {
		t.Fatal(err)
	}

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
				Unit:     "load",
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
}
