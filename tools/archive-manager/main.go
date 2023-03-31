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

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
)

func main() {
	var srcPath, flagConfigFile string

	flag.StringVar(&srcPath, "s", "./var/job-archive", "Specify the source job archive path. Default is ./var/job-archive")
	flag.StringVar(&flagConfigFile, "config", "./config.json", "Specify alternative path to `config.json`")
	flag.Parse()
	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", srcPath)

	config.Init(flagConfigFile)

	if err := archive.Init(json.RawMessage(archiveCfg)); err != nil {
		log.Fatal(err)
	}
	ar := archive.GetHandle()

	for jobMeta := range ar.Iter() {
		fmt.Printf("Validate %s - %d\n", jobMeta.Cluster, jobMeta.JobID)
	}
}
