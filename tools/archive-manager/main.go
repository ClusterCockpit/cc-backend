// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"encoding/json"
	"flag"
	"fmt"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
)

func main() {
	var srcPath, flagConfigFile, flagLogLevel string
	var flagLogDateTime bool

	flag.StringVar(&srcPath, "s", "./var/job-archive", "Specify the source job archive path. Default is ./var/job-archive")
	flag.BoolVar(&flagLogDateTime, "logdate", false, "Set this flag to add date and time to log messages")
	flag.StringVar(&flagLogLevel, "loglevel", "warn", "Sets the logging level: `[debug,info,warn (default),err,fatal,crit]`")
	flag.StringVar(&flagConfigFile, "config", "./config.json", "Specify alternative path to `config.json`")
	flag.Parse()
	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", srcPath)

	log.Init(flagLogLevel, flagLogDateTime)
	config.Init(flagConfigFile)
	config.Keys.Validate = true

	if err := archive.Init(json.RawMessage(archiveCfg), false); err != nil {
		log.Fatal(err)
	}
	ar := archive.GetHandle()

	for job := range ar.Iter(true) {
		log.Printf("Validate %s - %d\n", job.Meta.Cluster, job.Meta.JobID)
	}
}
