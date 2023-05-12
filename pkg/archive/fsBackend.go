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
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
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

func checkFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !errors.Is(err, os.ErrNotExist)
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
		fmt.Println("Err")
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

func (fsa *FsArchive) Exists(job *schema.Job) bool {
	dir := getDirectory(job, fsa.path)
	_, err := os.Stat(dir)
	return !errors.Is(err, os.ErrNotExist)
}

func (fsa *FsArchive) CleanUp(jobs []*schema.Job) {
	for _, job := range jobs {
		dir := getDirectory(job, fsa.path)
		if err := os.RemoveAll(dir); err != nil {
			log.Errorf("JobArchive Cleanup() error: %v", err)
		}
	}
}

func (fsa *FsArchive) Compress(jobs []*schema.Job) {
	for _, job := range jobs {
		fileIn := getPath(job, fsa.path, "data.json")
		if !checkFileExists(fileIn) && (job.Duration > 600 || job.NumNodes > 4) {

			originalFile, err := os.Open(fileIn)
			if err != nil {
				log.Errorf("JobArchive Compress() error: %v", err)
			}
			defer originalFile.Close()

			fileOut := getPath(job, fsa.path, "data.json.gz")
			gzippedFile, err := os.Create(fileOut)

			if err != nil {
				log.Errorf("JobArchive Compress() error: %v", err)
			}
			defer gzippedFile.Close()

			gzipWriter := gzip.NewWriter(gzippedFile)
			defer gzipWriter.Close()

			_, err = io.Copy(gzipWriter, originalFile)
			if err != nil {
				log.Errorf("JobArchive Compress() error: %v", err)
			}
			gzipWriter.Flush()
			if err := os.Remove(fileIn); err != nil {
				log.Errorf("JobArchive Compress() error: %v", err)
			}
		}
	}
}

func (fsa *FsArchive) LoadJobData(job *schema.Job) (schema.JobData, error) {
	var isCompressed bool = true
	filename := getPath(job, fsa.path, "data.json.gz")
	if !checkFileExists(filename) {
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

								if !checkFileExists(filename) {
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
