package metricdata

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/ClusterCockpit/cc-jobarchive/config"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/ClusterCockpit/cc-jobarchive/schema"
)

// For a given job, return the path of the `data.json`/`meta.json` file.
// TODO: Implement Issue ClusterCockpit/ClusterCockpit#97
func getPath(job *model.Job, file string, checkLegacy bool) (string, error) {
	lvl1, lvl2 := fmt.Sprintf("%d", job.JobID/1000), fmt.Sprintf("%03d", job.JobID%1000)
	if !checkLegacy {
		return filepath.Join(JobArchivePath, job.Cluster, lvl1, lvl2, strconv.FormatInt(job.StartTime.Unix(), 10), file), nil
	}

	legacyPath := filepath.Join(JobArchivePath, job.Cluster, lvl1, lvl2, file)
	if _, err := os.Stat(legacyPath); errors.Is(err, os.ErrNotExist) {
		return filepath.Join(JobArchivePath, job.Cluster, lvl1, lvl2, strconv.FormatInt(job.StartTime.Unix(), 10), file), nil
	}

	return legacyPath, nil
}

// Assuming job is completed/archived, return the jobs metric data.
func loadFromArchive(job *model.Job) (schema.JobData, error) {
	filename, err := getPath(job, "data.json", true)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var data schema.JobData
	if err := json.NewDecoder(bufio.NewReader(f)).Decode(&data); err != nil {
		return nil, err
	}

	return data, nil
}

// If the job is archived, find its `meta.json` file and override the tags list
// in that JSON file. If the job is not archived, nothing is done.
func UpdateTags(job *model.Job, tags []*model.JobTag) error {
	if job.State == model.JobStateRunning {
		return nil
	}

	filename, err := getPath(job, "meta.json", true)
	if err != nil {
		return err
	}

	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var metaFile schema.JobMeta
	if err := json.NewDecoder(f).Decode(&metaFile); err != nil {
		return err
	}
	f.Close()

	metaFile.Tags = make([]struct {
		Name string "json:\"Name\""
		Type string "json:\"Type\""
	}, 0)
	for _, tag := range tags {
		metaFile.Tags = append(metaFile.Tags, struct {
			Name string "json:\"Name\""
			Type string "json:\"Type\""
		}{
			Name: tag.TagName,
			Type: tag.TagType,
		})
	}

	bytes, err := json.Marshal(metaFile)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, bytes, 0644)
}

// Helper to metricdata.LoadAverages().
func loadAveragesFromArchive(job *model.Job, metrics []string, data [][]schema.Float) error {
	filename, err := getPath(job, "meta.json", true)
	if err != nil {
		return err
	}

	bytes, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var metaFile schema.JobMeta
	if err := json.Unmarshal(bytes, &metaFile); err != nil {
		return err
	}

	for i, m := range metrics {
		if stat, ok := metaFile.Statistics[m]; ok {
			data[i] = append(data[i], schema.Float(stat.Avg))
		} else {
			data[i] = append(data[i], schema.NaN)
		}
	}

	return nil
}

// Writes a running job to the job-archive
func ArchiveJob(job *model.Job, ctx context.Context) (*schema.JobMeta, error) {
	if job.State != model.JobStateRunning {
		return nil, errors.New("cannot archive job that is not running")
	}

	allMetrics := make([]string, 0)
	metricConfigs := config.GetClusterConfig(job.Cluster).MetricConfig
	for _, mc := range metricConfigs {
		allMetrics = append(allMetrics, mc.Name)
	}
	jobData, err := LoadData(job, allMetrics, ctx)
	if err != nil {
		return nil, err
	}

	tags := []struct {
		Name string `json:"Name"`
		Type string `json:"Type"`
	}{}
	for _, tag := range job.Tags {
		tags = append(tags, struct {
			Name string `json:"Name"`
			Type string `json:"Type"`
		}{
			Name: tag.TagName,
			Type: tag.TagType,
		})
	}

	metaData := &schema.JobMeta{
		JobId:            int64(job.JobID),
		User:             job.User,
		Project:          job.Project,
		Cluster:          job.Cluster,
		NumNodes:         job.NumNodes,
		NumHWThreads:     job.NumHWThreads,
		NumAcc:           job.NumAcc,
		Exclusive:        int8(job.Exclusive),
		MonitoringStatus: int8(job.MonitoringStatus),
		SMT:              int8(job.Smt),
		Partition:        job.Partition,
		ArrayJobId:       job.ArrayJobID,
		JobState:         string(job.State),
		StartTime:        job.StartTime.Unix(),
		Duration:         int64(job.Duration),
		Resources:        job.Resources,
		MetaData:         "", // TODO/FIXME: Handle `meta_data`!
		Tags:             tags,
		Statistics:       make(map[string]*schema.JobMetaStatistics),
	}

	for metric, data := range jobData {
		avg, min, max := 0.0, math.MaxFloat32, -math.MaxFloat32
		for _, nodedata := range data.Series {
			avg += nodedata.Statistics.Avg
			min = math.Min(min, nodedata.Statistics.Min)
			max = math.Max(max, nodedata.Statistics.Max)
		}

		metaData.Statistics[metric] = &schema.JobMetaStatistics{
			Unit: config.GetMetricConfig(job.Cluster, metric).Unit,
			Avg:  avg / float64(job.NumNodes),
			Min:  min,
			Max:  max,
		}
	}

	// If the file based archive is disabled,
	// only return the JobMeta structure as the
	// statistics in there are needed.
	if !useArchive {
		return metaData, nil
	}

	dirPath, err := getPath(job, "", false)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dirPath, 0777); err != nil {
		return nil, err
	}

	f, err := os.Create(path.Join(dirPath, "meta.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	writer := bufio.NewWriter(f)
	if err := json.NewEncoder(writer).Encode(metaData); err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}

	f, err = os.Create(path.Join(dirPath, "data.json"))
	if err != nil {
		return nil, err
	}
	writer = bufio.NewWriter(f)
	if err := json.NewEncoder(writer).Encode(jobData); err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}

	return metaData, f.Close()
}
