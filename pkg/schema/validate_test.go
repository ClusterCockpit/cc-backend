// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package schema

import (
	"bytes"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	json := []byte(`{
    "jwts": {
        "max-age": "2m"
    },
	"clusters": [
	{
	   "name": "testcluster",
	   "metricDataRepository": {
		"kind": "cc-metric-store",
		 "url": "localhost:8082"},
	   "filterRanges": {
		"numNodes": { "from": 1, "to": 64 },
		"duration": { "from": 0, "to": 86400 },
		"startTime": { "from": "2022-01-01T00:00:00Z", "to": null }
	}}]
}`)

	if err := Validate(Config, bytes.NewReader(json)); err != nil {
		t.Errorf("Error is not nil! %v", err)
	}
}

func TestValidateJobMeta(t *testing.T) {

}

func TestValidateCluster(t *testing.T) {
	json := []byte(`{
		"name": "emmy",
		"subClusters": [
			{
				"name": "main",
				"processorType": "Intel IvyBridge",
				"socketsPerNode": 2,
				"coresPerSocket": 10,
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
                "nodes": "w11[27-45,49-63,69-72]",
				"topology": {
					"node": [0,20,1,21,2,22,3,23,4,24,5,25,6,26,7,27,8,28,9,29,10,30,11,31,12,32,13,33,14,34,15,35,16,36,17,37,18,38,19,39],
					"socket": [
						[0,20,1,21,2,22,3,23,4,24,5,25,6,26,7,27,8,28,9,29],
						[10,30,11,31,12,32,13,33,14,34,15,35,16,36,17,37,18,38,19,39]
					],
					"memoryDomain": [
						[0,20,1,21,2,22,3,23,4,24,5,25,6,26,7,27,8,28,9,29],
						[10,30,11,31,12,32,13,33,14,34,15,35,16,36,17,37,18,38,19,39]
					],
					"core": [
						[0,20],[1,21],[2,22],[3,23],[4,24],[5,25],[6,26],[7,27],[8,28],[9,29],[10,30],[11,31],[12,32],[13,33],[14,34],[15,35],[16,36],[17,37],[18,38],[19,39]
					]
				}
			}
		],
		"metricConfig": [
			{
				"name": "cpu_load",
				"scope": "hwthread",
				"unit": {"base": ""},
                "aggregation": "avg",
				"timestep": 60,
			    "peak": 4,
                "normal": 2,
                "caution": 1,
                "alert": 0.25
			}
		]
}`)

	if err := Validate(ClusterCfg, bytes.NewReader(json)); err != nil {
		t.Errorf("Error is not nil! %v", err)
	}
}
