// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/archiver"
	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph"
	"github.com/ClusterCockpit/cc-backend/internal/importer"
	"github.com/ClusterCockpit/cc-backend/internal/metricstore"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	ccconf "github.com/ClusterCockpit/cc-lib/v2/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	lp "github.com/ClusterCockpit/cc-lib/v2/ccMessage"
	"github.com/ClusterCockpit/cc-lib/v2/schema"

	_ "github.com/mattn/go-sqlite3"
)

func setupNatsTest(t *testing.T) *NatsAPI {
	repository.ResetConnection()

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
	const testclusterJSON = `{
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

	if err := os.WriteFile(filepath.Join(jobarchive, "version.txt"), fmt.Appendf(nil, "%d", 3), 0o666); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(jobarchive, "testcluster"), 0o777); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(jobarchive, "testcluster", "cluster.json"), []byte(testclusterJSON), 0o666); err != nil {
		t.Fatal(err)
	}

	dbfilepath := filepath.Join(tmpdir, "test.db")
	err := repository.MigrateDB(dbfilepath)
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

	// metricstore initialization removed - it's initialized via callback in tests

	archiver.Start(repository.GetJobRepository(), context.Background())

	if cfg := ccconf.GetPackageConfig("auth"); cfg != nil {
		auth.Init(&cfg)
	} else {
		cclog.Warn("Authentication disabled due to missing configuration")
		auth.Init(nil)
	}

	graph.Init()

	return NewNatsAPI()
}

func cleanupNatsTest() {
	if err := archiver.Shutdown(5 * time.Second); err != nil {
		cclog.Warnf("Archiver shutdown timeout in tests: %v", err)
	}
}

func TestNatsHandleStartJob(t *testing.T) {
	natsAPI := setupNatsTest(t)
	t.Cleanup(cleanupNatsTest)

	tests := []struct {
		name          string
		payload       string
		expectError   bool
		validateJob   func(t *testing.T, job *schema.Job)
		shouldFindJob bool
	}{
		{
			name: "valid job start",
			payload: `{
				"jobId": 1001,
				"user": "testuser1",
				"project": "testproj1",
				"cluster": "testcluster",
				"partition": "main",
				"walltime": 7200,
				"numNodes": 1,
				"numHwthreads": 8,
				"numAcc": 0,
				"shared": "none",
				"monitoringStatus": 1,
				"smt": 1,
				"resources": [
					{
						"hostname": "host123",
						"hwthreads": [0, 1, 2, 3, 4, 5, 6, 7]
					}
				],
				"startTime": 1234567890
			}`,
			expectError:   false,
			shouldFindJob: true,
			validateJob: func(t *testing.T, job *schema.Job) {
				if job.JobID != 1001 {
					t.Errorf("expected JobID 1001, got %d", job.JobID)
				}
				if job.User != "testuser1" {
					t.Errorf("expected user testuser1, got %s", job.User)
				}
				if job.State != schema.JobStateRunning {
					t.Errorf("expected state running, got %s", job.State)
				}
			},
		},
		{
			name: "invalid JSON",
			payload: `{
				"jobId": "not a number",
				"user": "testuser2"
			}`,
			expectError:   true,
			shouldFindJob: false,
		},
		{
			name: "missing required fields",
			payload: `{
				"jobId": 1002
			}`,
			expectError:   true,
			shouldFindJob: false,
		},
		{
			name: "job with unknown fields (should fail due to DisallowUnknownFields)",
			payload: `{
				"jobId": 1003,
				"user": "testuser3",
				"project": "testproj3",
				"cluster": "testcluster",
				"partition": "main",
				"walltime": 3600,
				"numNodes": 1,
				"numHwthreads": 8,
				"unknownField": "should cause error",
				"startTime": 1234567900
			}`,
			expectError:   true,
			shouldFindJob: false,
		},
		{
			name: "job with tags",
			payload: `{
				"jobId": 1004,
				"user": "testuser4",
				"project": "testproj4",
				"cluster": "testcluster",
				"partition": "main",
				"walltime": 3600,
				"numNodes": 1,
				"numHwthreads": 8,
				"numAcc": 0,
				"shared": "none",
				"monitoringStatus": 1,
				"smt": 1,
				"resources": [
					{
						"hostname": "host123",
						"hwthreads": [0, 1, 2, 3]
					}
				],
				"tags": [
					{
						"type": "test",
						"name": "testtag",
						"scope": "testuser4"
					}
				],
				"startTime": 1234567910
			}`,
			expectError:   false,
			shouldFindJob: true,
			validateJob: func(t *testing.T, job *schema.Job) {
				if job.JobID != 1004 {
					t.Errorf("expected JobID 1004, got %d", job.JobID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			natsAPI.handleStartJob(tt.payload)
			natsAPI.JobRepository.SyncJobs()

			// Allow some time for async operations
			time.Sleep(100 * time.Millisecond)

			if tt.shouldFindJob {
				// Extract jobId from payload
				var payloadMap map[string]any
				json.Unmarshal([]byte(tt.payload), &payloadMap)
				jobID := int64(payloadMap["jobId"].(float64))
				cluster := payloadMap["cluster"].(string)
				startTime := int64(payloadMap["startTime"].(float64))

				job, err := natsAPI.JobRepository.Find(&jobID, &cluster, &startTime)
				if err != nil {
					if !tt.expectError {
						t.Fatalf("expected to find job, but got error: %v", err)
					}
					return
				}

				if tt.validateJob != nil {
					tt.validateJob(t, job)
				}
			}
		})
	}
}

func TestNatsHandleStopJob(t *testing.T) {
	natsAPI := setupNatsTest(t)
	t.Cleanup(cleanupNatsTest)

	// First, create a running job
	startPayload := `{
		"jobId": 2001,
		"user": "testuser",
		"project": "testproj",
		"cluster": "testcluster",
		"partition": "main",
		"walltime": 3600,
		"numNodes": 1,
		"numHwthreads": 8,
		"numAcc": 0,
		"shared": "none",
		"monitoringStatus": 1,
		"smt": 1,
		"resources": [
			{
				"hostname": "host123",
				"hwthreads": [0, 1, 2, 3, 4, 5, 6, 7]
			}
		],
		"startTime": 1234567890
	}`

	natsAPI.handleStartJob(startPayload)
	natsAPI.JobRepository.SyncJobs()
	time.Sleep(100 * time.Millisecond)

	tests := []struct {
		name         string
		payload      string
		expectError  bool
		validateJob  func(t *testing.T, job *schema.Job)
		setupJobFunc func() // Optional: create specific test job
	}{
		{
			name: "valid job stop - completed",
			payload: `{
				"jobId": 2001,
				"cluster": "testcluster",
				"startTime": 1234567890,
				"jobState": "completed",
				"stopTime": 1234571490
			}`,
			expectError: false,
			validateJob: func(t *testing.T, job *schema.Job) {
				if job.State != schema.JobStateCompleted {
					t.Errorf("expected state completed, got %s", job.State)
				}
				expectedDuration := int32(1234571490 - 1234567890)
				if job.Duration != expectedDuration {
					t.Errorf("expected duration %d, got %d", expectedDuration, job.Duration)
				}
			},
		},
		{
			name: "valid job stop - failed",
			setupJobFunc: func() {
				startPayloadFailed := `{
					"jobId": 2002,
					"user": "testuser",
					"project": "testproj",
					"cluster": "testcluster",
					"partition": "main",
					"walltime": 3600,
					"numNodes": 1,
					"numHwthreads": 8,
					"numAcc": 0,
					"shared": "none",
					"monitoringStatus": 1,
					"smt": 1,
					"resources": [
						{
							"hostname": "host123",
							"hwthreads": [0, 1, 2, 3]
						}
					],
					"startTime": 1234567900
				}`
				natsAPI.handleStartJob(startPayloadFailed)
				natsAPI.JobRepository.SyncJobs()
				time.Sleep(100 * time.Millisecond)
			},
			payload: `{
				"jobId": 2002,
				"cluster": "testcluster",
				"startTime": 1234567900,
				"jobState": "failed",
				"stopTime": 1234569900
			}`,
			expectError: false,
			validateJob: func(t *testing.T, job *schema.Job) {
				if job.State != schema.JobStateFailed {
					t.Errorf("expected state failed, got %s", job.State)
				}
			},
		},
		{
			name: "invalid JSON",
			payload: `{
				"jobId": "not a number"
			}`,
			expectError: true,
		},
		{
			name: "missing jobId",
			payload: `{
				"cluster": "testcluster",
				"jobState": "completed",
				"stopTime": 1234571490
			}`,
			expectError: true,
		},
		{
			name: "invalid job state",
			setupJobFunc: func() {
				startPayloadInvalid := `{
					"jobId": 2003,
					"user": "testuser",
					"project": "testproj",
					"cluster": "testcluster",
					"partition": "main",
					"walltime": 3600,
					"numNodes": 1,
					"numHwthreads": 8,
					"numAcc": 0,
					"shared": "none",
					"monitoringStatus": 1,
					"smt": 1,
					"resources": [
						{
							"hostname": "host123",
							"hwthreads": [0, 1]
						}
					],
					"startTime": 1234567910
				}`
				natsAPI.handleStartJob(startPayloadInvalid)
				natsAPI.JobRepository.SyncJobs()
				time.Sleep(100 * time.Millisecond)
			},
			payload: `{
				"jobId": 2003,
				"cluster": "testcluster",
				"startTime": 1234567910,
				"jobState": "invalid_state",
				"stopTime": 1234571510
			}`,
			expectError: true,
		},
		{
			name: "stopTime before startTime",
			setupJobFunc: func() {
				startPayloadTime := `{
					"jobId": 2004,
					"user": "testuser",
					"project": "testproj",
					"cluster": "testcluster",
					"partition": "main",
					"walltime": 3600,
					"numNodes": 1,
					"numHwthreads": 8,
					"numAcc": 0,
					"shared": "none",
					"monitoringStatus": 1,
					"smt": 1,
					"resources": [
						{
							"hostname": "host123",
							"hwthreads": [0]
						}
					],
					"startTime": 1234567920
				}`
				natsAPI.handleStartJob(startPayloadTime)
				natsAPI.JobRepository.SyncJobs()
				time.Sleep(100 * time.Millisecond)
			},
			payload: `{
				"jobId": 2004,
				"cluster": "testcluster",
				"startTime": 1234567920,
				"jobState": "completed",
				"stopTime": 1234567900
			}`,
			expectError: true,
		},
		{
			name: "job not found",
			payload: `{
				"jobId": 99999,
				"cluster": "testcluster",
				"startTime": 1234567890,
				"jobState": "completed",
				"stopTime": 1234571490
			}`,
			expectError: true,
		},
	}

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

	metricstore.TestLoadDataCallback = func(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context, resolution int) (schema.JobData, error) {
		return testData, nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupJobFunc != nil {
				tt.setupJobFunc()
			}

			natsAPI.handleStopJob(tt.payload)

			// Allow some time for async operations
			time.Sleep(100 * time.Millisecond)

			if !tt.expectError && tt.validateJob != nil {
				// Extract job details from payload
				var payloadMap map[string]any
				json.Unmarshal([]byte(tt.payload), &payloadMap)
				jobID := int64(payloadMap["jobId"].(float64))
				cluster := payloadMap["cluster"].(string)

				var startTime *int64
				if st, ok := payloadMap["startTime"]; ok {
					t := int64(st.(float64))
					startTime = &t
				}

				job, err := natsAPI.JobRepository.Find(&jobID, &cluster, startTime)
				if err != nil {
					t.Fatalf("expected to find job, but got error: %v", err)
				}

				tt.validateJob(t, job)
			}
		})
	}
}

func TestNatsHandleNodeState(t *testing.T) {
	natsAPI := setupNatsTest(t)
	t.Cleanup(cleanupNatsTest)

	tests := []struct {
		name        string
		payload     string
		expectError bool
		validateFn  func(t *testing.T)
	}{
		{
			name: "valid node state update",
			payload: `{
				"cluster": "testcluster",
				"nodes": [
					{
						"hostname": "host123",
						"states": ["allocated"],
						"cpusAllocated": 8,
						"memoryAllocated": 16384,
						"gpusAllocated": 0,
						"jobsRunning": 1
					}
				]
			}`,
			expectError: false,
			validateFn: func(t *testing.T) {
				// In a full test, we would verify the node state was updated in the database
				// For now, just ensure no error occurred
			},
		},
		{
			name: "multiple nodes",
			payload: `{
				"cluster": "testcluster",
				"nodes": [
					{
						"hostname": "host123",
						"states": ["idle"],
						"cpusAllocated": 0,
						"memoryAllocated": 0,
						"gpusAllocated": 0,
						"jobsRunning": 0
					},
					{
						"hostname": "host124",
						"states": ["allocated"],
						"cpusAllocated": 4,
						"memoryAllocated": 8192,
						"gpusAllocated": 1,
						"jobsRunning": 1
					}
				]
			}`,
			expectError: false,
		},
		{
			name: "invalid JSON",
			payload: `{
				"cluster": "testcluster",
				"nodes": "not an array"
			}`,
			expectError: true,
		},
		{
			name: "empty nodes array",
			payload: `{
				"cluster": "testcluster",
				"nodes": []
			}`,
			expectError: false, // Empty array should not cause error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			natsAPI.handleNodeState("test.subject", []byte(tt.payload))

			// Allow some time for async operations
			time.Sleep(50 * time.Millisecond)

			if tt.validateFn != nil {
				tt.validateFn(t)
			}
		})
	}
}

func TestNatsProcessJobEvent(t *testing.T) {
	natsAPI := setupNatsTest(t)
	t.Cleanup(cleanupNatsTest)

	msgStartJob, err := lp.NewMessage(
		"job",
		map[string]string{"function": "start_job"},
		nil,
		map[string]any{
			"event": `{
				"jobId": 3001,
				"user": "testuser",
				"project": "testproj",
				"cluster": "testcluster",
				"partition": "main",
				"walltime": 3600,
				"numNodes": 1,
				"numHwthreads": 8,
				"numAcc": 0,
				"shared": "none",
				"monitoringStatus": 1,
				"smt": 1,
				"resources": [
					{
						"hostname": "host123",
						"hwthreads": [0, 1, 2, 3]
					}
				],
				"startTime": 1234567890
			}`,
		},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to create test message: %v", err)
	}

	msgMissingTag, err := lp.NewMessage(
		"job",
		map[string]string{},
		nil,
		map[string]any{
			"event": `{}`,
		},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to create test message: %v", err)
	}

	msgUnknownFunc, err := lp.NewMessage(
		"job",
		map[string]string{"function": "unknown_function"},
		nil,
		map[string]any{
			"event": `{}`,
		},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("failed to create test message: %v", err)
	}

	tests := []struct {
		name        string
		message     lp.CCMessage
		expectError bool
	}{
		{
			name:        "start_job function",
			message:     msgStartJob,
			expectError: false,
		},
		{
			name:        "missing function tag",
			message:     msgMissingTag,
			expectError: true,
		},
		{
			name:        "unknown function",
			message:     msgUnknownFunc,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			natsAPI.processJobEvent(tt.message)
			time.Sleep(50 * time.Millisecond)
		})
	}
}

func TestNatsHandleJobEvent(t *testing.T) {
	natsAPI := setupNatsTest(t)
	t.Cleanup(cleanupNatsTest)

	tests := []struct {
		name        string
		data        []byte
		expectError bool
	}{
		{
			name:        "valid influx line protocol",
			data:        []byte(`job,function=start_job event="{\"jobId\":4001,\"user\":\"testuser\",\"project\":\"testproj\",\"cluster\":\"testcluster\",\"partition\":\"main\",\"walltime\":3600,\"numNodes\":1,\"numHwthreads\":8,\"numAcc\":0,\"shared\":\"none\",\"monitoringStatus\":1,\"smt\":1,\"resources\":[{\"hostname\":\"host123\",\"hwthreads\":[0,1,2,3]}],\"startTime\":1234567890}"`),
			expectError: false,
		},
		{
			name:        "invalid influx line protocol",
			data:        []byte(`invalid line protocol format`),
			expectError: true,
		},
		{
			name:        "empty data",
			data:        []byte(``),
			expectError: false, // Decoder should handle empty input gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// HandleJobEvent doesn't return errors, it logs them
			// We're just ensuring it doesn't panic
			natsAPI.handleJobEvent("test.subject", tt.data)
			time.Sleep(50 * time.Millisecond)
		})
	}
}

func TestNatsHandleStartJobDuplicatePrevention(t *testing.T) {
	natsAPI := setupNatsTest(t)
	t.Cleanup(cleanupNatsTest)

	// Start a job
	payload := `{
		"jobId": 5001,
		"user": "testuser",
		"project": "testproj",
		"cluster": "testcluster",
		"partition": "main",
		"walltime": 3600,
		"numNodes": 1,
		"numHwthreads": 8,
		"numAcc": 0,
		"shared": "none",
		"monitoringStatus": 1,
		"smt": 1,
		"resources": [
			{
				"hostname": "host123",
				"hwthreads": [0, 1, 2, 3]
			}
		],
		"startTime": 1234567890
	}`

	natsAPI.handleStartJob(payload)
	natsAPI.JobRepository.SyncJobs()
	time.Sleep(100 * time.Millisecond)

	// Try to start the same job again (within 24 hours)
	duplicatePayload := `{
		"jobId": 5001,
		"user": "testuser",
		"project": "testproj",
		"cluster": "testcluster",
		"partition": "main",
		"walltime": 3600,
		"numNodes": 1,
		"numHwthreads": 8,
		"numAcc": 0,
		"shared": "none",
		"monitoringStatus": 1,
		"smt": 1,
		"resources": [
			{
				"hostname": "host123",
				"hwthreads": [0, 1, 2, 3]
			}
		],
		"startTime": 1234567900
	}`

	natsAPI.handleStartJob(duplicatePayload)
	natsAPI.JobRepository.SyncJobs()
	time.Sleep(100 * time.Millisecond)

	// Verify only one job exists
	jobID := int64(5001)
	cluster := "testcluster"
	jobs, err := natsAPI.JobRepository.FindAll(&jobID, &cluster, nil)
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(jobs))
	}
}
