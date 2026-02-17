// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
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
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// FsArchiveConfig holds the configuration for the filesystem archive backend.
type FsArchiveConfig struct {
	Path string `json:"path"` // Root directory path for the archive
}

// FsArchive implements ArchiveBackend using a hierarchical filesystem structure.
// Jobs are stored in directories organized by cluster, job ID, and start time.
//
// Directory structure: <path>/<cluster>/<jobid/1000>/<jobid%1000>/<starttime>/
type FsArchive struct {
	path     string   // Root path of the archive
	clusters []string // List of discovered cluster names
}

// clusterInfo holds statistics about jobs in a cluster.
type clusterInfo struct {
	numJobs   int     // Total number of jobs
	dateFirst int64   // Unix timestamp of oldest job
	dateLast  int64   // Unix timestamp of newest job
	diskSize  float64 // Total disk usage in MB
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
		strconv.FormatInt(job.StartTime, 10))
}

func getPath(
	job *schema.Job,
	rootPath string,
	file string,
) string {
	return filepath.Join(
		getDirectory(job, rootPath), file)
}

func loadJobMeta(filename string) (*schema.Job, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		cclog.Errorf("loadJobMeta() > open file error: %v", err)
		return nil, err
	}
	if config.Keys.Validate {
		if err := schema.Validate(schema.Meta, bytes.NewReader(b)); err != nil {
			return nil, fmt.Errorf("validate job meta: %v", err)
		}
	}

	return DecodeJobMeta(bytes.NewReader(b))
}

func loadJobData(filename string, isCompressed bool) (schema.JobData, error) {
	f, err := os.Open(filename)
	if err != nil {
		cclog.Errorf("fsBackend LoadJobData()- %v", err)
		return nil, err
	}
	defer f.Close()

	if isCompressed {
		r, err := gzip.NewReader(f)
		if err != nil {
			cclog.Errorf(" %v", err)
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

func loadJobStats(filename string, isCompressed bool) (schema.ScopedJobStats, error) {
	f, err := os.Open(filename)
	if err != nil {
		cclog.Errorf("fsBackend LoadJobStats()- %v", err)
		return nil, err
	}
	defer f.Close()

	if isCompressed {
		r, err := gzip.NewReader(f)
		if err != nil {
			cclog.Errorf(" %v", err)
			return nil, err
		}
		defer r.Close()

		if config.Keys.Validate {
			if err := schema.Validate(schema.Data, r); err != nil {
				return nil, fmt.Errorf("validate job data: %v", err)
			}
		}

		return DecodeJobStats(r, filename)
	} else {
		if config.Keys.Validate {
			if err := schema.Validate(schema.Data, bufio.NewReader(f)); err != nil {
				return nil, fmt.Errorf("validate job data: %v", err)
			}
		}
		return DecodeJobStats(bufio.NewReader(f), filename)
	}
}

func (fsa *FsArchive) Init(rawConfig json.RawMessage) (uint64, error) {
	var cfg FsArchiveConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		cclog.Warnf("Init() > Unmarshal error: %#v", err)
		return 0, err
	}
	if cfg.Path == "" {
		err := fmt.Errorf("Init() : empty config.Path")
		cclog.Errorf("Init() > config.Path error: %v", err)
		return 0, err
	}
	fsa.path = cfg.Path

	b, err := os.ReadFile(filepath.Join(fsa.path, "version.txt"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Check if directory is empty (ignoring hidden files/dirs)
			entries, err := os.ReadDir(fsa.path)
			if err != nil {
				cclog.Errorf("fsBackend Init() > ReadDir() error: %v", err)
				return 0, err
			}

			isEmpty := true
			for _, e := range entries {
				if e.Name()[0] != '.' {
					isEmpty = false
					break
				}
			}

			if isEmpty {
				cclog.Infof("fsBackend Init() > Bootstrapping new archive at %s", fsa.path)
				versionStr := fmt.Sprintf("%d\n", Version)
				if err := os.WriteFile(filepath.Join(fsa.path, "version.txt"), []byte(versionStr), 0o644); err != nil {
					cclog.Errorf("fsBackend Init() > failed to create version.txt: %v", err)
					return 0, err
				}
				return Version, nil
			}
		}

		cclog.Warnf("fsBackend Init() - %v", err)
		return 0, err
	}

	version, err := strconv.ParseUint(strings.TrimSuffix(string(b), "\n"), 10, 64)
	if err != nil {
		cclog.Errorf("fsBackend Init()- %v", err)
		return 0, err
	}

	if version != Version {
		return version, fmt.Errorf("unsupported version %d, need %d", version, Version)
	}

	entries, err := os.ReadDir(fsa.path)
	if err != nil {
		cclog.Errorf("Init() > ReadDir() error: %v", err)
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
		cclog.Fatalf("Reading clusters failed: %s", err.Error())
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
			cclog.Fatalf("Reading jobs failed @ lvl1 dirs: %s", err.Error())
		}

		for _, lvl1Dir := range lvl1Dirs {
			if !lvl1Dir.IsDir() {
				continue
			}
			lvl2Dirs, err := os.ReadDir(filepath.Join(fsa.path, cluster.Name(), lvl1Dir.Name()))
			if err != nil {
				cclog.Fatalf("Reading jobs failed @ lvl2 dirs: %s", err.Error())
			}

			for _, lvl2Dir := range lvl2Dirs {
				dirpath := filepath.Join(fsa.path, cluster.Name(), lvl1Dir.Name(), lvl2Dir.Name())
				startTimeDirs, err := os.ReadDir(dirpath)
				if err != nil {
					cclog.Fatalf("Reading jobs failed @ starttime dirs: %s", err.Error())
				}

				for _, startTimeDir := range startTimeDirs {
					if startTimeDir.IsDir() {
						ci[cc].numJobs++
						startTime, err := strconv.ParseInt(startTimeDir.Name(), 10, 64)
						if err != nil {
							cclog.Fatalf("Cannot parse starttime: %s", err.Error())
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
		cclog.Fatalf("Reading clusters failed: %s", err.Error())
	}

	for _, cluster := range clusters {
		if !cluster.IsDir() {
			continue
		}

		lvl1Dirs, err := os.ReadDir(filepath.Join(fsa.path, cluster.Name()))
		if err != nil {
			cclog.Fatalf("Reading jobs failed @ lvl1 dirs: %s", err.Error())
		}

		for _, lvl1Dir := range lvl1Dirs {
			if !lvl1Dir.IsDir() {
				continue
			}
			lvl2Dirs, err := os.ReadDir(filepath.Join(fsa.path, cluster.Name(), lvl1Dir.Name()))
			if err != nil {
				cclog.Fatalf("Reading jobs failed @ lvl2 dirs: %s", err.Error())
			}

			for _, lvl2Dir := range lvl2Dirs {
				dirpath := filepath.Join(fsa.path, cluster.Name(), lvl1Dir.Name(), lvl2Dir.Name())
				startTimeDirs, err := os.ReadDir(dirpath)
				if err != nil {
					cclog.Fatalf("Reading jobs failed @ starttime dirs: %s", err.Error())
				}

				for _, startTimeDir := range startTimeDirs {
					if startTimeDir.IsDir() {
						startTime, err := strconv.ParseInt(startTimeDir.Name(), 10, 64)
						if err != nil {
							cclog.Fatalf("Cannot parse starttime: %s", err.Error())
						}

						if startTime < before || startTime > after {
							if err := os.RemoveAll(filepath.Join(dirpath, startTimeDir.Name())); err != nil {
								cclog.Errorf("JobArchive Cleanup() error: %v", err)
							}
						}
					}
				}
				if util.GetFilecount(dirpath) == 0 {
					if err := os.Remove(dirpath); err != nil {
						cclog.Errorf("JobArchive Clean() error: %v", err)
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

		if err := os.MkdirAll(filepath.Clean(filepath.Join(target, "..")), 0o777); err != nil {
			cclog.Errorf("JobArchive Move MkDir error: %v", err)
		}
		if err := os.Rename(source, target); err != nil {
			cclog.Errorf("JobArchive Move() error: %v", err)
		}

		parent := filepath.Clean(filepath.Join(source, ".."))
		if util.GetFilecount(parent) == 0 {
			if err := os.Remove(parent); err != nil {
				cclog.Errorf("JobArchive Move() error: %v", err)
			}
		}
	}
}

func (fsa *FsArchive) CleanUp(jobs []*schema.Job) {
	start := time.Now()
	for _, job := range jobs {
		if job == nil {
			cclog.Errorf("JobArchive Cleanup() error: job is nil")
			continue
		}
		dir := getDirectory(job, fsa.path)
		if err := os.RemoveAll(dir); err != nil {
			cclog.Errorf("JobArchive Cleanup() error: %v", err)
		}

		parent := filepath.Clean(filepath.Join(dir, ".."))
		if util.GetFilecount(parent) == 0 {
			if err := os.Remove(parent); err != nil {
				cclog.Errorf("JobArchive Cleanup() error: %v", err)
			}
		}
	}

	cclog.Infof("Retention Service - Remove %d files in %s", len(jobs), time.Since(start))
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

	cclog.Infof("Compression Service - %d files took %s", cnt, time.Since(start))
}

func (fsa *FsArchive) CompressLast(starttime int64) int64 {
	filename := filepath.Join(fsa.path, "compress.txt")
	b, err := os.ReadFile(filename)
	if err != nil {
		cclog.Errorf("fsBackend Compress - %v", err)
		os.WriteFile(filename, fmt.Appendf(nil, "%d", starttime), 0o644)
		return starttime
	}
	last, err := strconv.ParseInt(strings.TrimSuffix(string(b), "\n"), 10, 64)
	if err != nil {
		cclog.Errorf("fsBackend Compress - %v", err)
		return starttime
	}

	cclog.Infof("fsBackend Compress - start %d last %d", starttime, last)
	os.WriteFile(filename, fmt.Appendf(nil, "%d", starttime), 0o644)
	return last
}

func (fsa *FsArchive) LoadJobData(job *schema.Job) (schema.JobData, error) {
	isCompressed := true
	filename := getPath(job, fsa.path, "data.json.gz")

	if !util.CheckFileExists(filename) {
		filename = getPath(job, fsa.path, "data.json")
		isCompressed = false
	}

	return loadJobData(filename, isCompressed)
}

func (fsa *FsArchive) LoadJobStats(job *schema.Job) (schema.ScopedJobStats, error) {
	isCompressed := true
	filename := getPath(job, fsa.path, "data.json.gz")

	if !util.CheckFileExists(filename) {
		filename = getPath(job, fsa.path, "data.json")
		isCompressed = false
	}

	return loadJobStats(filename, isCompressed)
}

func (fsa *FsArchive) LoadJobMeta(job *schema.Job) (*schema.Job, error) {
	filename := getPath(job, fsa.path, "meta.json")
	return loadJobMeta(filename)
}

func (fsa *FsArchive) LoadClusterCfg(name string) (*schema.Cluster, error) {
	b, err := os.ReadFile(filepath.Join(fsa.path, name, "cluster.json"))
	if err != nil {
		cclog.Errorf("LoadClusterCfg() > open file error: %v", err)
		return &schema.Cluster{}, err
	}

	if config.Keys.Validate {
		if err := schema.Validate(schema.ClusterCfg, bytes.NewReader(b)); err != nil {
			cclog.Warnf("Validate cluster config: %v\n", err)
			return &schema.Cluster{}, fmt.Errorf("validate cluster config: %v", err)
		}
	}
	return DecodeCluster(bytes.NewReader(b))
}

func (fsa *FsArchive) Iter(loadMetricData bool) <-chan JobContainer {
	ch := make(chan JobContainer)

	go func() {
		defer close(ch)

		numWorkers := 4
		jobPaths := make(chan string, numWorkers*2)
		var wg sync.WaitGroup

		for range numWorkers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for jobPath := range jobPaths {
					job, err := loadJobMeta(filepath.Join(jobPath, "meta.json"))
					if err != nil && !errors.Is(err, &jsonschema.ValidationError{}) {
						cclog.Errorf("in %s: %s", jobPath, err.Error())
						continue
					}

					if loadMetricData {
						isCompressed := true
						filename := filepath.Join(jobPath, "data.json.gz")

						if !util.CheckFileExists(filename) {
							filename = filepath.Join(jobPath, "data.json")
							isCompressed = false
						}

						data, err := loadJobData(filename, isCompressed)
						if err != nil && !errors.Is(err, &jsonschema.ValidationError{}) {
							cclog.Errorf("in %s: %s", jobPath, err.Error())
						}
						ch <- JobContainer{Meta: job, Data: &data}
					} else {
						ch <- JobContainer{Meta: job, Data: nil}
					}
				}
			}()
		}

		clustersDir, err := os.ReadDir(fsa.path)
		if err != nil {
			cclog.Fatalf("Reading clusters failed @ cluster dirs: %s", err.Error())
		}

		for _, clusterDir := range clustersDir {
			if !clusterDir.IsDir() {
				continue
			}
			lvl1Dirs, err := os.ReadDir(filepath.Join(fsa.path, clusterDir.Name()))
			if err != nil {
				cclog.Fatalf("Reading jobs failed @ lvl1 dirs: %s", err.Error())
			}

			for _, lvl1Dir := range lvl1Dirs {
				if !lvl1Dir.IsDir() {
					continue
				}

				lvl2Dirs, err := os.ReadDir(filepath.Join(fsa.path, clusterDir.Name(), lvl1Dir.Name()))
				if err != nil {
					cclog.Fatalf("Reading jobs failed @ lvl2 dirs: %s", err.Error())
				}

				for _, lvl2Dir := range lvl2Dirs {
					dirpath := filepath.Join(fsa.path, clusterDir.Name(), lvl1Dir.Name(), lvl2Dir.Name())
					startTimeDirs, err := os.ReadDir(dirpath)
					if err != nil {
						cclog.Fatalf("Reading jobs failed @ starttime dirs: %s", err.Error())
					}

					for _, startTimeDir := range startTimeDirs {
						if startTimeDir.IsDir() {
							jobPaths <- filepath.Join(dirpath, startTimeDir.Name())
						}
					}
				}
			}
		}

		close(jobPaths)
		wg.Wait()
	}()

	return ch
}

func (fsa *FsArchive) StoreJobMeta(job *schema.Job) error {
	f, err := os.Create(getPath(job, fsa.path, "meta.json"))
	if err != nil {
		cclog.Error("Error while creating filepath for meta.json")
		return err
	}
	if err := EncodeJobMeta(f, job); err != nil {
		cclog.Error("Error while encoding job metadata to meta.json file")
		return err
	}
	if err := f.Close(); err != nil {
		cclog.Warn("Error while closing meta.json file")
		return err
	}

	return nil
}

func (fsa *FsArchive) GetClusters() []string {
	return fsa.clusters
}

func (fsa *FsArchive) ImportJob(
	jobMeta *schema.Job,
	jobData *schema.JobData,
) error {
	dir := getPath(jobMeta, fsa.path, "")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		cclog.Error("Error while creating job archive path")
		return err
	}

	f, err := os.Create(path.Join(dir, "meta.json"))
	if err != nil {
		cclog.Error("Error while creating filepath for meta.json")
		return err
	}
	if err := EncodeJobMeta(f, jobMeta); err != nil {
		cclog.Error("Error while encoding job metadata to meta.json file")
		return err
	}
	if err := f.Close(); err != nil {
		cclog.Warn("Error while closing meta.json file")
		return err
	}

	var dataBuf bytes.Buffer
	if err := EncodeJobData(&dataBuf, jobData); err != nil {
		cclog.Error("Error while encoding job metricdata")
		return err
	}

	if dataBuf.Len() > 2000 {
		f, err = os.Create(path.Join(dir, "data.json.gz"))
		if err != nil {
			cclog.Error("Error while creating filepath for data.json.gz")
			return err
		}
		gzipWriter := gzip.NewWriter(f)
		if _, err := gzipWriter.Write(dataBuf.Bytes()); err != nil {
			cclog.Error("Error while writing compressed job data")
			gzipWriter.Close()
			f.Close()
			return err
		}
		if err := gzipWriter.Close(); err != nil {
			cclog.Warn("Error while closing gzip writer")
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			cclog.Warn("Error while closing data.json.gz file")
			return err
		}
	} else {
		f, err = os.Create(path.Join(dir, "data.json"))
		if err != nil {
			cclog.Error("Error while creating filepath for data.json")
			return err
		}
		if _, err := f.Write(dataBuf.Bytes()); err != nil {
			cclog.Error("Error while writing job metricdata to data.json file")
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			cclog.Warn("Error while closing data.json file")
			return err
		}
	}

	return nil
}

func (fsa *FsArchive) StoreClusterCfg(name string, config *schema.Cluster) error {
	dir := filepath.Join(fsa.path, name)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		cclog.Errorf("StoreClusterCfg() > mkdir error: %v", err)
		return err
	}

	f, err := os.Create(filepath.Join(dir, "cluster.json"))
	if err != nil {
		cclog.Errorf("StoreClusterCfg() > create file error: %v", err)
		return err
	}
	defer f.Close()

	if err := EncodeCluster(f, config); err != nil {
		cclog.Errorf("StoreClusterCfg() > encode error: %v", err)
		return err
	}

	// Update clusters list if new
	found := slices.Contains(fsa.clusters, name)
	if !found {
		fsa.clusters = append(fsa.clusters, name)
	}

	return nil
}
