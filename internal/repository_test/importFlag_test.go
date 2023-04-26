// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
)

type Result struct {
	JobId     int64
	Cluster   string
	StartTime int64
	Duration  int32
}

func readResult(t *testing.T, testname string) Result {
	var r Result

	content, err := os.ReadFile(filepath.Join("testdata",
		fmt.Sprintf("%s-golden.json", testname)))
	if err != nil {
		t.Fatal("Error when opening file: ", err)
	}

	err = json.Unmarshal(content, &r)
	if err != nil {
		t.Fatal("Error during Unmarshal(): ", err)
	}

	return r
}

func TestHandleImportFlag(t *testing.T) {
	r := setupRepo(t)

	tests, err := filepath.Glob(filepath.Join("testdata", "*.input"))
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range tests {
		_, filename := filepath.Split(path)
		str := strings.Split(strings.TrimSuffix(filename, ".input"), "-")
		testname := str[1]

		t.Run(testname, func(t *testing.T) {
			s := fmt.Sprintf("%s:%s", filepath.Join("testdata",
				fmt.Sprintf("meta-%s.input", testname)),
				filepath.Join("testdata", fmt.Sprintf("data-%s.json", testname)))
			err := repository.HandleImportFlag(s)
			if err != nil {
				t.Fatal(err)
			}

			result := readResult(t, testname)
			job, err := r.Find(&result.JobId, &result.Cluster, &result.StartTime)
			if err != nil {
				t.Fatal(err)
			}
			if job.Duration != result.Duration {
				t.Errorf("wrong duration for job\ngot: %d \nwant: %d", job.Duration, result.Duration)
			}

		})
	}
}
