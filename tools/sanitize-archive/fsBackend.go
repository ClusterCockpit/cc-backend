// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
)

type FsArchiveConfig struct {
	Path string `json:"path"`
}

type FsArchive struct {
	path     string
	clusters []string
}

func getPath(
	job *JobMeta,
	rootPath string,
	file string) string {

	lvl1, lvl2 := fmt.Sprintf("%d", job.JobID/1000), fmt.Sprintf("%03d", job.JobID%1000)
	return filepath.Join(
		rootPath,
		job.Cluster,
		lvl1, lvl2,
		strconv.FormatInt(job.StartTime, 10), file)
}

func loadJobMeta(filename string) (*JobMeta, error) {

	f, err := os.Open(filename)
	if err != nil {
		log.Errorf("fsBackend loadJobMeta()- %v", err)
		return &JobMeta{}, err
	}
	defer f.Close()

	return DecodeJobMeta(bufio.NewReader(f))
}

func (fsa *FsArchive) Init(rawConfig json.RawMessage) error {

	var config FsArchiveConfig
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		log.Errorf("fsBackend Init()- %v", err)
		return err
	}
	if config.Path == "" {
		err := fmt.Errorf("fsBackend Init()- empty path")
		log.Errorf("fsBackend Init()- %v", err)
		return err
	}
	fsa.path = config.Path

	entries, err := os.ReadDir(fsa.path)
	if err != nil {
		log.Errorf("fsBackend Init()- %v", err)
		return err
	}

	for _, de := range entries {
		fsa.clusters = append(fsa.clusters, de.Name())
	}

	return nil
}

func (fsa *FsArchive) LoadClusterCfg(name string) (*Cluster, error) {

	b, err := os.ReadFile(filepath.Join(fsa.path, name, "cluster.json"))
	if err != nil {
		log.Errorf("fsBackend LoadClusterCfg()- %v", err)
		return &Cluster{}, err
	}
	return DecodeCluster(bytes.NewReader(b))
}

func (fsa *FsArchive) Iter() <-chan *JobMeta {

	ch := make(chan *JobMeta)
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

					for _, startTimeDir := range startTimeDirs {
						if startTimeDir.IsDir() {
							job, err := loadJobMeta(filepath.Join(dirpath, startTimeDir.Name(), "meta.json"))
							if err != nil {
								log.Errorf("in %s: %s", filepath.Join(dirpath, startTimeDir.Name()), err.Error())
							} else {
								ch <- job
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

func (fsa *FsArchive) GetClusters() []string {

	return fsa.clusters
}
