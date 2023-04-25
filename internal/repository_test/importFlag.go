// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository_test

import "testing"

func TestHandleImportFlag(t *testing.T) {
	r := setupRepo(t)

  paths, err := filepath.Glob(filepath.Join("testdata", "*.input"))
  if err != nil {
    t.Fatal(err)
  }

  for _, path := range paths {
    _, filename := filepath.Split(path)
    testname := filename[:len(filename)-len(filepath.Ext(path))]

    // Each path turns into a test: the test name is the filename without the
    // extension.
    t.Run(testname, func(t *testing.T) {
      source, err := os.ReadFile(path)
      if err != nil {
        t.Fatal("error reading source file:", err)
      }

      // >>> This is the actual code under test.
      output, err := format.Source(source)
      if err != nil {
        t.Fatal("error formatting:", err)
      }
      // <<<

      // Each input file is expected to have a "golden output" file, with the
      // same path except the .input extension is replaced by .golden
      goldenfile := filepath.Join("testdata", testname+".golden")
      want, err := os.ReadFile(goldenfile)
      if err != nil {
        t.Fatal("error reading golden file:", err)
      }

      if !bytes.Equal(output, want) {
        t.Errorf("\n==== got:\n%s\n==== want:\n%s\n", output, want)
      }
    })
  }





	s := "../../test/repo/meta1.json:../../test/repo/data1.json"
	err := HandleImportFlag(s)
	if err != nil {
		t.Fatal(err)
	}

	jobId, cluster, startTime := int64(398764), "fritz", int64(1675954353)
	job, err := r.Find(&jobId, &cluster, &startTime)
	if err != nil {
		t.Fatal(err)
	}

	if job.ID != 2 {
		t.Errorf("wrong summary for diagnostic 3\ngot: %d \nwant: 1366", job.JobID)
	}
}
