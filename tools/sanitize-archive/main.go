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
	"os"

	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/ClusterCockpit/cc-backend/pkg/units"
)

var ar FsArchive

func deepCopyClusterConfig(co *Cluster) schema.Cluster {
	var cn schema.Cluster

	cn.Name = co.Name
	cn.SubClusters = co.SubClusters

	for _, mco := range co.MetricConfig {
		var mcn schema.MetricConfig
		mcn.Name = mco.Name
		mcn.Scope = mco.Scope
		mcn.Aggregation = mco.Aggregation
		mcn.Timestep = mco.Timestep
		mcn.Peak = mco.Peak
		mcn.Normal = mco.Normal
		mcn.Caution = mco.Caution
		mcn.Alert = mco.Alert
		mcn.Unit = units.ConvertUnitString(mco.Unit)
		cn.MetricConfig = append(cn.MetricConfig, &mcn)
	}

	return cn
}

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

	err = initClusterConfig()
	if err != nil {
		log.Fatal(err)
	}
	// setup new job archive
	err = os.Mkdir(dstPath, 0750)
	if err != nil {
		log.Fatal(err)
	}

	for _, c := range Clusters {
		path := fmt.Sprintf("%s/%s", dstPath, c.Name)
		fmt.Println(path)
		err = os.Mkdir(path, 0750)
		if err != nil {
			log.Fatal(err)
		}
		cn := deepCopyClusterConfig(c)

		f, err := os.Create(fmt.Sprintf("%s/%s/cluster.json", dstPath, c.Name))
		if err != nil {
			log.Fatal(err)
		}
		if err := EncodeCluster(f, &cn); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}

	// for job := range ar.Iter() {
	// 	fmt.Printf("Job %d\n", job.JobID)
	// }
}
