// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/util"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

type FsArchiveConfig struct {
	Path string `json:"path"`
}

type FsArchive struct {
	path     string
	clusters []string
}

type clusterInfo struct {
	numJobs   int
	dateFirst int64
	dateLast  int64
	diskSize  float64
}

func getDirectory(
	job *schema.Job,
	rootPath string,
) string {
	lvl1, lvl2 := fmt.Sprintf("%d", job.JobID/1000), fmt.Sprintf("%03d", job.JobID%1000)

	return filepath.Join(
		rootPath,
		job.Cluster,
		lvl1, lvl2,
		strconv.FormatInt(job.StartTime.Unix(), 10))
}

func getPath(
	job *schema.Job,
	rootPath string,
	file string) string {

	return filepath.Join(
		getDirectory(job, rootPath), file)
}

func loadJobMeta(filename string) (*schema.JobMeta, error) {

	b, err := os.ReadFile(filename)
	if err != nil {
		log.Errorf("loadJobMeta() > open file error: %v", err)
		return &schema.JobMeta{}, err
	}
	if config.Keys.Validate {
		if err := schema.Validate(schema.Meta, bytes.NewReader(b)); err != nil {
			return &schema.JobMeta{}, fmt.Errorf("validate job meta: %v", err)
		}
	}

	return DecodeJobMeta(bytes.NewReader(b))
}

func loadJobData(filename string, isCompressed bool) (schema.JobData, error) {
	f, err := os.Open(filename)

	if err != nil {
		log.Errorf("fsBackend LoadJobData()- %v", err)
		return nil, err
	}
	defer f.Close()

	if isCompressed {
		r, err := gzip.NewReader(f)
		if err != nil {
			log.Errorf(" %v", err)
			return nil, err
		}
		defer r.Close()

		if config.Keys.Validate {
			if err := schema.Validate(schema.Data, r); err != nil {
				return schema.JobData{}, fmt.Errorf("validate job data: %v", err)
			}
		}

		return DecodeJobData(r, filename)
	} else {
		if config.Keys.Validate {
			if err := schema.Validate(schema.Data, bufio.NewReader(f)); err != nil {
				return schema.JobData{}, fmt.Errorf("validate job data: %v", err)
			}
		}
		return DecodeJobData(bufio.NewReader(f), filename)
	}
}

func (fsa *FsArchive) Init(rawConfig json.RawMessage) (uint64, error) {

	var config FsArchiveConfig
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		log.Warnf("Init() > Unmarshal error: %#v", err)
		return 0, err
	}
	if config.Path == "" {
		err := fmt.Errorf("Init() : empty config.Path")
		log.Errorf("Init() > config.Path error: %v", err)
		return 0, err
	}
	fsa.path = config.Path

	b, err := os.ReadFile(filepath.Join(fsa.path, "version.txt"))
	if err != nil {
		log.Warnf("fsBackend Init() - %v", err)
		return 0, err
	}

	version, err := strconv.ParseUint(strings.TrimSuffix(string(b), "\n"), 10, 64)
	if err != nil {
		log.Errorf("fsBackend Init()- %v", err)
		return 0, err
	}

	if version != Version {
		return version, fmt.Errorf("unsupported version %d, need %d", version, Version)
	}

	entries, err := os.ReadDir(fsa.path)
	if err != nil {
		log.Errorf("Init() > ReadDir() error: %v", err)
		return 0, err
	}

	for _, de := range entries {
		if !de.IsDir() {
			continue
		}
		fsa.clusters = append(fsa.clusters, de.Name())
	}

	return version, nil
}

func (fsa *FsArchive) Info() {
	fmt.Printf("Job archive %s\n", fsa.path)
	clusters, err := os.ReadDir(fsa.path)
	if err != nil {
		log.Fatalf("Reading clusters failed: %s", err.Error())
	}

	ci := make(map[string]*clusterInfo)

	for _, cluster := range clusters {
		if !cluster.IsDir() {
			continue
		}

		cc := cluster.Name()
		ci[cc] = &clusterInfo{dateFirst: time.Now().Unix()}
		lvl1Dirs, err := os.ReadDir(filepath.Join(fsa.path, cluster.Name()))
		if err != nil {
			log.Fatalf("Reading jobs failed @ lvl1 dirs: %s", err.Error())
		}

		for _, lvl1Dir := range lvl1Dirs {
			if !lvl1Dir.IsDir() {
				continue
			}
			lvl2Dirs, err := os.ReadDir(filepath.Join(fsa.path, cluster.Name(), lvl1Dir.Name()))
			if err != nil {
				log.Fatalf("Reading jobs failed @ lvl2 dirs: %s", err.Error())
			}

			for _, lvl2Dir := range lvl2Dirs {
				dirpath := filepath.Join(fsa.path, cluster.Name(), lvl1Dir.Name(), lvl2Dir.Name())
				startTimeDirs, err := os.ReadDir(dirpath)
				if err != nil {
					log.Fatalf("Reading jobs failed @ starttime dirs: %s", err.Error())
				}

				for _, startTimeDir := range startTimeDirs {
					if startTimeDir.IsDir() {
						ci[cc].numJobs++
						startTime, err := strconv.ParseInt(startTimeDir.Name(), 10, 64)
						if err != nil {
							log.Fatalf("Cannot parse starttime: %s", err.Error())
						}
						ci[cc].dateFirst = util.Min(ci[cc].dateFirst, startTime)
						ci[cc].dateLast = util.Max(ci[cc].dateLast, startTime)
						ci[cc].diskSize += util.DiskUsage(filepath.Join(dirpath, startTimeDir.Name()))
					}
				}
			}
		}
	}

	cit := clusterInfo{dateFirst: time.Now().Unix()}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "cluster\t#jobs\tfrom\tto\tdu (MB)")
	for cluster, clusterInfo := range ci {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%.2f\n", cluster,
			clusterInfo.numJobs,
			time.Unix(clusterInfo.dateFirst, 0),
			time.Unix(clusterInfo.dateLast, 0),
			clusterInfo.diskSize)

		cit.numJobs += clusterInfo.numJobs
		cit.dateFirst = util.Min(cit.dateFirst, clusterInfo.dateFirst)
		cit.dateLast = util.Max(cit.dateLast, clusterInfo.dateLast)
		cit.diskSize += clusterInfo.diskSize
	}

	fmt.Fprintf(w, "TOTAL\t%d\t%s\t%s\t%.2f\n",
		cit.numJobs, time.Unix(cit.dateFirst, 0), time.Unix(cit.dateLast, 0), cit.diskSize)
	w.Flush()
}

func (fsa *FsArchive) Exists(job *schema.Job) bool {
	dir := getDirectory(job, fsa.path)
	_, err := os.Stat(dir)
	return !errors.Is(err, os.ErrNotExist)
}

func (fsa *FsArchive) Clean(before int64, after int64) {

	if after == 0 {
		after = math.MaxInt64
	}

	clusters, err := os.ReadDir(fsa.path)
	if err != nil {
		log.Fatalf("Reading clusters failed: %s", err.Error())
	}

	for _, cluster := range clusters {
		if !cluster.IsDir() {
			continue
		}

		lvl1Dirs, err := os.ReadDir(filepath.Join(fsa.path, cluster.Name()))
		if err != nil {
			log.Fatalf("Reading jobs failed @ lvl1 dirs: %s", err.Error())
		}

		for _, lvl1Dir := range lvl1Dirs {
			if !lvl1Dir.IsDir() {
				continue
			}
			lvl2Dirs, err := os.ReadDir(filepath.Join(fsa.path, cluster.Name(), lvl1Dir.Name()))
			if err != nil {
				log.Fatalf("Reading jobs failed @ lvl2 dirs: %s", err.Error())
			}

			for _, lvl2Dir := range lvl2Dirs {
				dirpath := filepath.Join(fsa.path, cluster.Name(), lvl1Dir.Name(), lvl2Dir.Name())
				startTimeDirs, err := os.ReadDir(dirpath)
				if err != nil {
					log.Fatalf("Reading jobs failed @ starttime dirs: %s", err.Error())
				}

				for _, startTimeDir := range startTimeDirs {
					if startTimeDir.IsDir() {
						startTime, err := strconv.ParseInt(startTimeDir.Name(), 10, 64)
						if err != nil {
							log.Fatalf("Cannot parse starttime: %s", err.Error())
						}

						if startTime < before || startTime > after {
							if err := os.RemoveAll(filepath.Join(dirpath, startTimeDir.Name())); err != nil {
								log.Errorf("JobArchive Cleanup() error: %v", err)
							}
						}
					}
				}
				if util.GetFilecount(dirpath) == 0 {
					if err := os.Remove(dirpath); err != nil {
						log.Errorf("JobArchive Clean() error: %v", err)
					}
				}
			}
		}
	}
}

func (fsa *FsArchive) Move(jobs []*schema.Job, path string) {
	for _, job := range jobs {
		source := getDirectory(job, fsa.path)
		target := getDirectory(job, path)

		if err := os.MkdirAll(filepath.Clean(filepath.Join(target, "..")), 0777); err != nil {
			log.Errorf("JobArchive Move MkDir error: %v", err)
		}
		if err := os.Rename(source, target); err != nil {
			log.Errorf("JobArchive Move() error: %v", err)
		}

		parent := filepath.Clean(filepath.Join(source, ".."))
		if util.GetFilecount(parent) == 0 {
			if err := os.Remove(parent); err != nil {
				log.Errorf("JobArchive Move() error: %v", err)
			}
		}
	}
}

func (fsa *FsArchive) CleanUp(jobs []*schema.Job) {
	start := time.Now()
	for _, job := range jobs {
		dir := getDirectory(job, fsa.path)
		if err := os.RemoveAll(dir); err != nil {
			log.Errorf("JobArchive Cleanup() error: %v", err)
		}

		parent := filepath.Clean(filepath.Join(dir, ".."))
		if util.GetFilecount(parent) == 0 {
			if err := os.Remove(parent); err != nil {
				log.Errorf("JobArchive Cleanup() error: %v", err)
			}
		}
	}

	log.Infof("Retention Service - Remove %d files in %s", len(jobs), time.Since(start))
}

func (fsa *FsArchive) Compress(jobs []*schema.Job) {
	var cnt int
	start := time.Now()

	for _, job := range jobs {
		fileIn := getPath(job, fsa.path, "data.json")
		if util.CheckFileExists(fileIn) && util.GetFilesize(fileIn) > 2000 {
			util.CompressFile(fileIn, getPath(job, fsa.path, "data.json.gz"))
			cnt++
		}
	}

	log.Infof("Compression Service - %d files took %s", cnt, time.Since(start))
}

func (fsa *FsArchive) CompressLast(starttime int64) int64 {

	filename := filepath.Join(fsa.path, "compress.txt")
	b, err := os.ReadFile(filename)
	if err != nil {
		log.Errorf("fsBackend Compress - %v", err)
		os.WriteFile(filename, []byte(fmt.Sprintf("%d", starttime)), 0644)
		return starttime
	}
	last, err := strconv.ParseInt(strings.TrimSuffix(string(b), "\n"), 10, 64)
	if err != nil {
		log.Errorf("fsBackend Compress - %v", err)
		return starttime
	}

	log.Infof("fsBackend Compress - start %d last %d", starttime, last)
	os.WriteFile(filename, []byte(fmt.Sprintf("%d", starttime)), 0644)
	return last
}

func (fsa *FsArchive) LoadJobData(job *schema.Job) (schema.JobData, error) {
	var isCompressed bool = true
	filename := getPath(job, fsa.path, "data.json.gz")

	if !util.CheckFileExists(filename) {
		filename = getPath(job, fsa.path, "data.json")
		isCompressed = false
	}

	return loadJobData(filename, isCompressed)
}

func (fsa *FsArchive) LoadJobMeta(job *schema.Job) (*schema.JobMeta, error) {
	filename := getPath(job, fsa.path, "meta.json")
	return loadJobMeta(filename)
}

func (fsa *FsArchive) LoadClusterCfg(name string) (*schema.Cluster, error) {

	b, err := os.ReadFile(filepath.Join(fsa.path, name, "cluster.json"))
	if err != nil {
		log.Errorf("LoadClusterCfg() > open file error: %v", err)
		// if config.Keys.Validate {
		if err := schema.Validate(schema.ClusterCfg, bytes.NewReader(b)); err != nil {
			log.Warnf("Validate cluster config: %v\n", err)
			return &schema.Cluster{}, fmt.Errorf("validate cluster config: %v", err)
		}
	}
	// }
	return DecodeCluster(bytes.NewReader(b))
}

func (fsa *FsArchive) Iter(loadMetricData bool) <-chan JobContainer {

	ch := make(chan JobContainer)
	go func() {
		clustersDir, err := os.ReadDir(fsa.path)
		if err != nil {
			log.Fatalf("Reading clusters failed @ cluster dirs: %s", err.Error())
		}

		for _, clusterDir := range clustersDir {
			if !clusterDir.IsDir() {
				continue
			}
			lvl1Dirs, err := os.ReadDir(filepath.Join(fsa.path, clusterDir.Name()))
			if err != nil {
				log.Fatalf("Reading jobs failed @ lvl1 dirs: %s", err.Error())
			}

			for _, lvl1Dir := range lvl1Dirs {
				if !lvl1Dir.IsDir() {
					// Could be the cluster.json file
					continue
				}

				lvl2Dirs, err := os.ReadDir(filepath.Join(fsa.path, clusterDir.Name(), lvl1Dir.Name()))
				if err != nil {
					log.Fatalf("Reading jobs failed @ lvl2 dirs: %s", err.Error())
				}

				for _, lvl2Dir := range lvl2Dirs {
					dirpath := filepath.Join(fsa.path, clusterDir.Name(), lvl1Dir.Name(), lvl2Dir.Name())
					startTimeDirs, err := os.ReadDir(dirpath)
					if err != nil {
						log.Fatalf("Reading jobs failed @ starttime dirs: %s", err.Error())
					}

					for _, startTimeDir := range startTimeDirs {
						if startTimeDir.IsDir() {
							job, err := loadJobMeta(filepath.Join(dirpath, startTimeDir.Name(), "meta.json"))
							if err != nil && !errors.Is(err, &jsonschema.ValidationError{}) {
								log.Errorf("in %s: %s", filepath.Join(dirpath, startTimeDir.Name()), err.Error())
							}

							if loadMetricData {
								var isCompressed bool = true
								filename := filepath.Join(dirpath, startTimeDir.Name(), "data.json.gz")

								if !util.CheckFileExists(filename) {
									filename = filepath.Join(dirpath, startTimeDir.Name(), "data.json")
									isCompressed = false
								}

								data, err := loadJobData(filename, isCompressed)
								if err != nil && !errors.Is(err, &jsonschema.ValidationError{}) {
									log.Errorf("in %s: %s", filepath.Join(dirpath, startTimeDir.Name()), err.Error())
								}
								ch <- JobContainer{Meta: job, Data: &data}
								log.Errorf("in %s: %s", filepath.Join(dirpath, startTimeDir.Name()), err.Error())
							} else {
								ch <- JobContainer{Meta: job, Data: nil}
							}
						}
					}
				}
			}
		}
		close(ch)
	}()
	return ch
}

func (fsa *FsArchive) StoreJobMeta(jobMeta *schema.JobMeta) error {

	job := schema.Job{
		BaseJob:       jobMeta.BaseJob,
		StartTime:     time.Unix(jobMeta.StartTime, 0),
		StartTimeUnix: jobMeta.StartTime,
	}
	f, err := os.Create(getPath(&job, fsa.path, "meta.json"))
	if err != nil {
		log.Error("Error while creating filepath for meta.json")
		return err
	}
	if err := EncodeJobMeta(f, jobMeta); err != nil {
		log.Error("Error while encoding job metadata to meta.json file")
		return err
	}
	if err := f.Close(); err != nil {
		log.Warn("Error while closing meta.json file")
		return err
	}

	return nil
}

func (fsa *FsArchive) GetClusters() []string {
	return fsa.clusters
}

func (fsa *FsArchive) ImportJob(
	jobMeta *schema.JobMeta,
	jobData *schema.JobData) error {

	job := schema.Job{
		BaseJob:       jobMeta.BaseJob,
		StartTime:     time.Unix(jobMeta.StartTime, 0),
		StartTimeUnix: jobMeta.StartTime,
	}
	dir := getPath(&job, fsa.path, "")
	if err := os.MkdirAll(dir, 0777); err != nil {
		log.Error("Error while creating job archive path")
		return err
	}

	f, err := os.Create(path.Join(dir, "meta.json"))
	if err != nil {
		log.Error("Error while creating filepath for meta.json")
		return err
	}
	if err := EncodeJobMeta(f, jobMeta); err != nil {
		log.Error("Error while encoding job metadata to meta.json file")
		return err
	}
	if err := f.Close(); err != nil {
		log.Warn("Error while closing meta.json file")
		return err
	}

	// var isCompressed bool = true
	// // TODO Use shortJob Config for check
	// if jobMeta.Duration < 300 {
	// 	isCompressed = false
	// 	f, err = os.Create(path.Join(dir, "data.json"))
	// } else {
	// 	f, err = os.Create(path.Join(dir, "data.json.gz"))
	// }
	// if err != nil {
	// 	return err
	// }
	//
	// if isCompressed {
	// 	if err := EncodeJobData(gzip.NewWriter(f), jobData); err != nil {
	// 		return err
	// 	}
	// } else {
	// 	if err := EncodeJobData(f, jobData); err != nil {
	// 		return err
	// 	}
	// }

	f, err = os.Create(path.Join(dir, "data.json"))
	if err != nil {
		log.Error("Error while creating filepath for data.json")
		return err
	}
	if err := EncodeJobData(f, jobData); err != nil {
		log.Error("Error while encoding job metricdata to data.json file")
		return err
	}
	if err := f.Close(); err != nil {
		log.Warn("Error while closing data.json file")
	}
	return err
}
