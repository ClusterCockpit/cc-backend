// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
)

func parseDate(in string) int64 {
	const shortForm = "2006-Jan-02"
	loc, _ := time.LoadLocation("Local")
	if in != "" {
		t, err := time.ParseInLocation(shortForm, in, loc)
		if err != nil {
			fmt.Printf("date parse error %v", err)
			os.Exit(0)
		}
		return t.Unix()
	}

	return 0
}

func main() {
	var srcPath, flagConfigFile, flagLogLevel, flagRemoveCluster, flagRemoveAfter, flagRemoveBefore string
	var flagLogDateTime, flagValidate bool

	flag.StringVar(&srcPath, "s", "./var/job-archive", "Specify the source job archive path. Default is ./var/job-archive")
	flag.BoolVar(&flagLogDateTime, "logdate", false, "Set this flag to add date and time to log messages")
	flag.StringVar(&flagLogLevel, "loglevel", "warn", "Sets the logging level: `[debug,info,warn (default),err,fatal,crit]`")
	flag.StringVar(&flagConfigFile, "config", "./config.json", "Specify alternative path to `config.json`")
	flag.StringVar(&flagRemoveCluster, "remove-cluster", "", "Remove cluster from archive and database")
	flag.StringVar(&flagRemoveBefore, "remove-before", "", "Remove all jobs with start time before date (Format: 2006-Jan-04)")
	flag.StringVar(&flagRemoveAfter, "remove-after", "", "Remove all jobs with start time after date (Format: 2006-Jan-04)")
	flag.BoolVar(&flagValidate, "validate", false, "Set this flag to validate a job archive against the json schema")
	flag.Parse()

	archiveCfg := fmt.Sprintf("{\"kind\": \"file\",\"path\": \"%s\"}", srcPath)

	log.Init(flagLogLevel, flagLogDateTime)
	config.Init(flagConfigFile)

	if err := archive.Init(json.RawMessage(archiveCfg), false); err != nil {
		log.Fatal(err)
	}
	ar := archive.GetHandle()

	if flagValidate {
		config.Keys.Validate = true
		for job := range ar.Iter(true) {
			log.Printf("Validate %s - %d\n", job.Meta.Cluster, job.Meta.JobID)
		}
		os.Exit(0)
	}

	if flagRemoveBefore != "" || flagRemoveAfter != "" {
		ar.Clean(parseDate(flagRemoveBefore), parseDate(flagRemoveAfter))
		os.Exit(0)
	}

	ar.Info()
}
