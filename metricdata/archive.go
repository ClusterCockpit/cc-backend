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
	"strings"

	"github.com/ClusterCockpit/cc-jobarchive/config"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/ClusterCockpit/cc-jobarchive/schema"
)

var JobArchivePath string = "./var/job-archive"

// For a given job, return the path of the `data.json`/`meta.json` file.
// TODO: Implement Issue ClusterCockpit/ClusterCockpit#97
func getPath(job *model.Job, file string) (string, error) {
	id, err := strconv.Atoi(strings.Split(job.JobID, ".")[0])
	if err != nil {
		return "", err
	}

	lvl1, lvl2 := fmt.Sprintf("%d", id/1000), fmt.Sprintf("%03d", id%1000)
	legacyPath := filepath.Join(JobArchivePath, job.ClusterID, lvl1, lvl2, file)
	if _, err := os.Stat(legacyPath); errors.Is(err, os.ErrNotExist) {
		return filepath.Join(JobArchivePath, job.ClusterID, lvl1, lvl2, strconv.FormatInt(job.StartTime.Unix(), 10), file), nil
	}

	return legacyPath, nil
}

// Assuming job is completed/archived, return the jobs metric data.
func loadFromArchive(job *model.Job) (schema.JobData, error) {
	filename, err := getPath(job, "data.json")
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

	filename, err := getPath(job, "meta.json")
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
		Name string "json:\"name\""
		Type string "json:\"type\""
	}, 0)
	for _, tag := range tags {
		metaFile.Tags = append(metaFile.Tags, struct {
			Name string "json:\"name\""
			Type string "json:\"type\""
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
	filename, err := getPath(job, "meta.json")
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
func ArchiveJob(job *model.Job, ctx context.Context) error {
	if job.State != model.JobStateRunning {
		return errors.New("cannot archive job that is not running")
	}

	allMetrics := make([]string, 0)
	metricConfigs := config.GetClusterConfig(job.ClusterID).MetricConfig
	for _, mc := range metricConfigs {
		allMetrics = append(allMetrics, mc.Name)
	}
	jobData, err := LoadData(job, allMetrics, ctx)
	if err != nil {
		return err
	}

	tags := []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}{}
	for _, tag := range job.Tags {
		tags = append(tags, struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}{
			Name: tag.TagName,
			Type: tag.TagType,
		})
	}

	metaData := &schema.JobMeta{
		JobId:      job.JobID,
		UserId:     job.UserID,
		ClusterId:  job.ClusterID,
		NumNodes:   job.NumNodes,
		JobState:   job.State.String(),
		StartTime:  job.StartTime.Unix(),
		Duration:   int64(job.Duration),
		Nodes:      job.Nodes,
		Tags:       tags,
		Statistics: make(map[string]*schema.JobMetaStatistics),
	}

	for metric, data := range jobData {
		avg, min, max := 0.0, math.MaxFloat32, -math.MaxFloat32
		for _, nodedata := range data.Series {
			avg += nodedata.Statistics.Avg
			min = math.Min(min, nodedata.Statistics.Min)
			max = math.Max(max, nodedata.Statistics.Max)
		}

		metaData.Statistics[metric] = &schema.JobMetaStatistics{
			Unit: config.GetMetricConfig(job.ClusterID, metric).Unit,
			Avg:  avg / float64(job.NumNodes),
			Min:  min,
			Max:  max,
		}
	}

	dirPath, err := getPath(job, "")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dirPath, 0777); err != nil {
		return err
	}

	f, err := os.Create(path.Join(dirPath, "meta.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	writer := bufio.NewWriter(f)
	if err := json.NewEncoder(writer).Encode(metaData); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	f, err = os.Create(path.Join(dirPath, "data.json"))
	if err != nil {
		return err
	}
	writer = bufio.NewWriter(f)
	if err := json.NewEncoder(writer).Encode(metaData); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	return f.Close()
}
