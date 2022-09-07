// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

type FsArchiveConfig struct {
	Path string `json:"path"`
}

type FsArchive struct {
	path     string
	clusters []string
}

// For a given job, return the path of the `data.json`/`meta.json` file.
// TODO: Implement Issue ClusterCockpit/ClusterCockpit#97
func getPath(
	job *schema.Job,
	rootPath string,
	file string) string {

	lvl1, lvl2 := fmt.Sprintf("%d", job.JobID/1000), fmt.Sprintf("%03d", job.JobID%1000)
	return filepath.Join(
		rootPath,
		job.Cluster,
		lvl1, lvl2,
		strconv.FormatInt(job.StartTime.Unix(), 10), file)
}

func loadJobMeta(filename string) (schema.JobMeta, error) {

	f, err := os.Open(filename)
	if err != nil {
		return schema.JobMeta{}, err
	}
	defer f.Close()

	return DecodeJobMeta(bufio.NewReader(f))
}

func (fsa *FsArchive) Init(rawConfig json.RawMessage) error {

	var config FsArchiveConfig
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return fmt.Errorf("fsBackend Init()- %w", err)
	}
	fsa.path = config.Path

	entries, err := os.ReadDir(fsa.path)
	if err != nil {
		return fmt.Errorf("fsBackend Init()- Cannot read dir %s: %w", fsa.path, err)
	}

	for _, de := range entries {
		fsa.clusters = append(fsa.clusters, de.Name())
	}

	return nil
}

func (fsa *FsArchive) LoadJobData(job *schema.Job) (schema.JobData, error) {

	filename := getPath(job, fsa.path, "data.json")

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return DecodeJobData(bufio.NewReader(f))
}

func (fsa *FsArchive) LoadJobMeta(job *schema.Job) (schema.JobMeta, error) {

	filename := getPath(job, fsa.path, "meta.json")

	f, err := os.Open(filename)
	if err != nil {
		return schema.JobMeta{}, err
	}
	defer f.Close()

	return DecodeJobMeta(bufio.NewReader(f))
}

func (fsa *FsArchive) LoadClusterCfg(name string) (schema.Cluster, error) {

	f, err := os.Open(filepath.Join(fsa.path, name, "cluster.json"))
	if err != nil {
		return schema.Cluster{}, fmt.Errorf("fsBackend LoadClusterCfg()- Cannot open %s: %w", name, err)
	}
	defer f.Close()

	return DecodeCluster(bufio.NewReader(f))
}

func (fsa *FsArchive) Iter() <-chan *schema.JobMeta {

	ch := make(chan *schema.JobMeta)
	go func() {
		clustersDir, err := os.ReadDir(fsa.path)
		if err != nil {
			log.Fatalf("Reading clusters failed: %s", err.Error())
		}

		for _, clusterDir := range clustersDir {
			lvl1Dirs, err := os.ReadDir(filepath.Join(fsa.path, clusterDir.Name()))
			if err != nil {
				log.Fatalf("Reading jobs failed: %s", err.Error())
			}

			for _, lvl1Dir := range lvl1Dirs {
				if !lvl1Dir.IsDir() {
					// Could be the cluster.json file
					continue
				}

				lvl2Dirs, err := os.ReadDir(filepath.Join(fsa.path, clusterDir.Name(), lvl1Dir.Name()))
				if err != nil {
					log.Fatalf("Reading jobs failed: %s", err.Error())
				}

				for _, lvl2Dir := range lvl2Dirs {
					dirpath := filepath.Join(fsa.path, clusterDir.Name(), lvl1Dir.Name(), lvl2Dir.Name())
					startTimeDirs, err := os.ReadDir(dirpath)
					if err != nil {
						log.Fatalf("Reading jobs failed: %s", err.Error())
					}

					// For compability with the old job-archive directory structure where
					// there was no start time directory.
					for _, startTimeDir := range startTimeDirs {
						if startTimeDir.IsDir() {
							job, err := loadJobMeta(filepath.Join(dirpath, startTimeDir.Name()))

							if err != nil {
								log.Errorf("in %s: %s", filepath.Join(dirpath, startTimeDir.Name()), err.Error())
							} else {
								ch <- &job
							}
						}
					}
				}
			}
		}
	}()
	return ch
}

func (fsa *FsArchive) StoreMeta(jobMeta *schema.JobMeta) error {

	job := schema.Job{
		BaseJob:       jobMeta.BaseJob,
		StartTime:     time.Unix(jobMeta.StartTime, 0),
		StartTimeUnix: jobMeta.StartTime,
	}
	f, err := os.Create(getPath(&job, fsa.path, "meta.json"))
	if err != nil {
		return err
	}
	if err := EncodeJobMeta(f, jobMeta); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	return nil
}

func (fsa *FsArchive) GetClusters() []string {

	return fsa.clusters
}

func (fsa *FsArchive) Import(
	jobMeta *schema.JobMeta,
	jobData *schema.JobData) error {

	job := schema.Job{
		BaseJob:       jobMeta.BaseJob,
		StartTime:     time.Unix(jobMeta.StartTime, 0),
		StartTimeUnix: jobMeta.StartTime,
	}
	dir := getPath(&job, fsa.path, "")
	if err := os.MkdirAll(dir, 0777); err != nil {
		return err
	}

	f, err := os.Create(path.Join(dir, "meta.json"))
	if err != nil {
		return err
	}
	if err := EncodeJobMeta(f, jobMeta); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	f, err = os.Create(path.Join(dir, "data.json"))
	if err != nil {
		return err
	}
	if err := EncodeJobData(f, jobData); err != nil {
		return err
	}
	return f.Close()
}
