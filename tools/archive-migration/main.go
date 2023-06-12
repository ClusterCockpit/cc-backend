// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	ccunits "github.com/ClusterCockpit/cc-units"
)

const Version = 1

var ar FsArchive
var srcPath string
var dstPath string

func loadJobData(filename string) (*JobData, error) {

	f, err := os.Open(filename)
	if err != nil {
		return &JobData{}, fmt.Errorf("fsBackend loadJobData()- %v", err)
	}
	defer f.Close()

	return DecodeJobData(bufio.NewReader(f))
}

func ConvertUnitString(us string) schema.Unit {
	var nu schema.Unit

	if us == "CPI" ||
		us == "IPC" ||
		us == "load" ||
		us == "" {
		nu.Base = us
		return nu
	}
	u := ccunits.NewUnit(us)
	p := u.GetPrefix()
	if p.Prefix() != "" {
		prefix := p.Prefix()
		nu.Prefix = prefix
	}
	m := u.GetMeasure()
	d := u.GetUnitDenominator()
	if d.Short() != "inval" {
		nu.Base = fmt.Sprintf("%s/%s", m.Short(), d.Short())
	} else {
		nu.Base = m.Short()
	}

	return nu
}

func deepCopyJobMeta(j *JobMeta) schema.JobMeta {
	var jn schema.JobMeta

	//required properties
	jn.JobID = j.JobID
	jn.User = j.User
	jn.Project = j.Project
	jn.Cluster = j.Cluster
	jn.SubCluster = j.SubCluster
	jn.NumNodes = j.NumNodes
	jn.Exclusive = j.Exclusive
	jn.StartTime = j.StartTime
	jn.State = schema.JobState(j.State)
	jn.Duration = j.Duration

	for _, ro := range j.Resources {
		var rn schema.Resource
		rn.Hostname = ro.Hostname
		rn.Configuration = ro.Configuration
		hwt := make([]int, len(ro.HWThreads))
		if ro.HWThreads != nil {
			copy(hwt, ro.HWThreads)
		}
		rn.HWThreads = hwt
		acc := make([]string, len(ro.Accelerators))
		if ro.Accelerators != nil {
			copy(acc, ro.Accelerators)
		}
		rn.Accelerators = acc
		jn.Resources = append(jn.Resources, &rn)
	}
	jn.MetaData = make(map[string]string)

	for k, v := range j.MetaData {
		jn.MetaData[k] = v
	}

	jn.Statistics = make(map[string]schema.JobStatistics)
	for k, v := range j.Statistics {
		var sn schema.JobStatistics
		sn.Avg = v.Avg
		sn.Max = v.Max
		sn.Min = v.Min
		tmpUnit := ConvertUnitString(v.Unit)
		if tmpUnit.Base == "inval" {
			sn.Unit = schema.Unit{Base: ""}
		} else {
			sn.Unit = tmpUnit
		}
		jn.Statistics[k] = sn
	}

	//optional properties
	jn.Partition = j.Partition
	jn.ArrayJobId = j.ArrayJobId
	jn.NumHWThreads = j.NumHWThreads
	jn.NumAcc = j.NumAcc
	jn.MonitoringStatus = j.MonitoringStatus
	jn.SMT = j.SMT
	jn.Walltime = j.Walltime

	for _, t := range j.Tags {
		jn.Tags = append(jn.Tags, t)
	}

	return jn
}

func deepCopyJobData(d *JobData, cluster string, subCluster string) *schema.JobData {
	var dn = make(schema.JobData)

	for k, v := range *d {
		// fmt.Printf("Metric %s\n", k)
		dn[k] = make(map[schema.MetricScope]*schema.JobMetric)

		for mk, mv := range v {
			// fmt.Printf("Scope %s\n", mk)
			var mn schema.JobMetric
			tmpUnit := ConvertUnitString(mv.Unit)
			if tmpUnit.Base == "inval" {
				mn.Unit = schema.Unit{Base: ""}
			} else {
				mn.Unit = tmpUnit
			}

			mn.Timestep = mv.Timestep

			for _, v := range mv.Series {
				var sn schema.Series
				sn.Hostname = v.Hostname
				if v.Id != nil {
					var id = new(string)

					if mk == schema.MetricScopeAccelerator {
						s := GetSubCluster(cluster, subCluster)
						var err error

						*id, err = s.Topology.GetAcceleratorID(*v.Id)
						if err != nil {
							log.Fatal(err)
						}

					} else {
						*id = fmt.Sprint(*v.Id)
					}
					sn.Id = id
				}
				if v.Statistics != nil {
					sn.Statistics = schema.MetricStatistics{
						Avg: v.Statistics.Avg,
						Min: v.Statistics.Min,
						Max: v.Statistics.Max}
				}

				sn.Data = make([]schema.Float, len(v.Data))
				copy(sn.Data, v.Data)
				mn.Series = append(mn.Series, sn)
			}

			dn[k][mk] = &mn
		}
		// fmt.Printf("FINISH %s\n", k)
	}

	return &dn
}

func deepCopyClusterConfig(co *Cluster) schema.Cluster {
	var cn schema.Cluster

	cn.Name = co.Name
	for _, sco := range co.SubClusters {
		var scn schema.SubCluster
		scn.Name = sco.Name
		scn.Nodes = sco.Nodes
		scn.ProcessorType = sco.ProcessorType
		scn.SocketsPerNode = sco.SocketsPerNode
		scn.CoresPerSocket = sco.CoresPerSocket
		scn.ThreadsPerCore = sco.ThreadsPerCore
		scn.FlopRateScalar = schema.MetricValue{
			Unit:  schema.Unit{Base: "F/s", Prefix: "G"},
			Value: float64(sco.FlopRateScalar)}
		scn.FlopRateSimd = schema.MetricValue{
			Unit:  schema.Unit{Base: "F/s", Prefix: "G"},
			Value: float64(sco.FlopRateSimd)}
		scn.MemoryBandwidth = schema.MetricValue{
			Unit:  schema.Unit{Base: "B/s", Prefix: "G"},
			Value: float64(sco.MemoryBandwidth)}
		scn.Topology = *sco.Topology
		cn.SubClusters = append(cn.SubClusters, &scn)
	}

	for _, mco := range co.MetricConfig {
		var mcn schema.MetricConfig
		mcn.Name = mco.Name
		mcn.Scope = mco.Scope
		if mco.Aggregation == "" {
			fmt.Println("cluster.json - Property aggregation missing! Please review file!")
			mcn.Aggregation = "sum"
		} else {
			mcn.Aggregation = mco.Aggregation
		}
		mcn.Timestep = mco.Timestep
		tmpUnit := ConvertUnitString(mco.Unit)
		if tmpUnit.Base == "inval" {
			mcn.Unit = schema.Unit{Base: ""}
		} else {
			mcn.Unit = tmpUnit
		}
		mcn.Peak = mco.Peak
		mcn.Normal = mco.Normal
		mcn.Caution = mco.Caution
		mcn.Alert = mco.Alert
		mcn.SubClusters = mco.SubClusters

		cn.MetricConfig = append(cn.MetricConfig, &mcn)
	}

	return cn
}

func convertJob(job *JobMeta) {
	// check if source data is available, otherwise skip job
	src_data_path := getPath(job, srcPath, "data.json")
	info, err := os.Stat(src_data_path)
	if err != nil {
		log.Fatal(err)
	}
	if info.Size() == 0 {
		fmt.Printf("Skip path %s, filesize is 0 Bytes.", src_data_path)
		return
	}

	path := getPath(job, dstPath, "meta.json")
	err = os.MkdirAll(filepath.Dir(path), 0750)
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}

	jmn := deepCopyJobMeta(job)
	if err = EncodeJobMeta(f, &jmn); err != nil {
		log.Fatal(err)
	}
	if err = f.Close(); err != nil {
		log.Fatal(err)
	}

	f, err = os.Create(getPath(job, dstPath, "data.json"))
	if err != nil {
		log.Fatal(err)
	}

	var jd *JobData
	jd, err = loadJobData(src_data_path)
	if err != nil {
		log.Fatal(err)
	}
	jdn := deepCopyJobData(jd, job.Cluster, job.SubCluster)
	if err := EncodeJobData(f, jdn); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	var flagLogLevel, flagConfigFile string
	var flagLogDateTime, debug bool

	flag.BoolVar(&flagLogDateTime, "logdate", false, "Set this flag to add date and time to log messages")
	flag.BoolVar(&debug, "debug", false, "Set this flag to force sequential execution for debugging")
	flag.StringVar(&flagLogLevel, "loglevel", "warn", "Sets the logging level: `[debug,info,warn (default),err,fatal,crit]`")
	flag.StringVar(&flagConfigFile, "config", "./config.json", "Specify alternative path to `config.json`")
	flag.StringVar(&srcPath, "src", "./var/job-archive", "Specify the source job archive path")
	flag.StringVar(&dstPath, "dst", "./var/job-archive-new", "Specify the destination job archive path")
	flag.Parse()

	if _, err := os.Stat(filepath.Join(srcPath, "version.txt")); !errors.Is(err, os.ErrNotExist) {
		log.Fatal("Archive version exists!")
	}

	log.Init(flagLogLevel, flagLogDateTime)
	config.Init(flagConfigFile)
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

	var wg sync.WaitGroup

	for job := range ar.Iter() {
		if debug {
			fmt.Printf("Job %d\n", job.JobID)
			convertJob(job)
		} else {
			job := job
			wg.Add(1)

			go func() {
				defer wg.Done()
				convertJob(job)
			}()
		}
	}

	wg.Wait()
	os.WriteFile(filepath.Join(dstPath, "version.txt"), []byte(fmt.Sprintf("%d", Version)), 0644)
}
