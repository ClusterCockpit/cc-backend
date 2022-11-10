// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
)

var ar FsArchive

func main() {
	var srcPath string
	var dstPath string

	flag.StringVar(&srcPath, "s", "./var/job-archive", "Specify the source job archive path. Default is ./var/job-archive")
	flag.StringVar(&dstPath, "d", "./var/job-archive-new", "Specify the destination job archive path. Default is ./var/job-archive-new")

	srcConfig := fmt.Sprintf("{\"path\": \"%s\"}", srcPath)
	err := ar.Init(json.RawMessage(srcConfig))
	if err != nil {
		log.Fatal(err)
	}

	for job := range ar.Iter() {
		fmt.Printf("Job %d\n", job.JobID)
	}
}
