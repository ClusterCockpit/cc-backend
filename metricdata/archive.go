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
	"github.com/ClusterCockpit/cc-jobarchive/schema"
)

// For a given job, return the path of the `data.json`/`meta.json` file.
// TODO: Implement Issue ClusterCockpit/ClusterCockpit#97
func getPath(job *schema.Job, file string, checkLegacy bool) (string, error) {
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
func loadFromArchive(job *schema.Job) (schema.JobData, error) {
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
func UpdateTags(job *schema.Job, tags []*schema.Tag) error {
	if job.State == schema.JobStateRunning {
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

	var metaFile schema.JobMeta = schema.JobMeta{
		BaseJob: schema.JobDefaults,
	}
	if err := json.NewDecoder(f).Decode(&metaFile); err != nil {
		return err
	}
	f.Close()

	metaFile.Tags = make([]*schema.Tag, 0)
	for _, tag := range tags {
		metaFile.Tags = append(metaFile.Tags, &schema.Tag{
			Name: tag.Name,
			Type: tag.Type,
		})
	}

	bytes, err := json.Marshal(metaFile)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, bytes, 0644)
}

// Helper to metricdata.LoadAverages().
func loadAveragesFromArchive(job *schema.Job, metrics []string, data [][]schema.Float) error {
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
func ArchiveJob(job *schema.Job, ctx context.Context) (*schema.JobMeta, error) {
	if job.State != schema.JobStateRunning {
		return nil, errors.New("cannot archive job that is not running")
	}

	allMetrics := make([]string, 0)
	metricConfigs := config.GetClusterConfig(job.Cluster).MetricConfig
	for _, mc := range metricConfigs {
		allMetrics = append(allMetrics, mc.Name)
	}

	// TODO: Use more granular resolution on non-exclusive jobs?
	scopes := []schema.MetricScope{schema.MetricScopeNode}
	jobData, err := LoadData(job, allMetrics, scopes, ctx)
	if err != nil {
		return nil, err
	}

	if err := calcStatisticsSeries(job, jobData); err != nil {
		return nil, err
	}

	jobMeta := &schema.JobMeta{
		BaseJob:    job.BaseJob,
		StartTime:  job.StartTime.Unix(),
		Statistics: make(map[string]schema.JobStatistics),
	}

	for metric, data := range jobData {
		avg, min, max := 0.0, math.MaxFloat32, -math.MaxFloat32
		nodeData, ok := data["node"]
		if !ok {
			// TODO/FIXME: Calc average for non-node metrics as well!
			continue
		}

		for _, series := range nodeData.Series {
			avg += series.Statistics.Avg
			min = math.Min(min, series.Statistics.Min)
			max = math.Max(max, series.Statistics.Max)
		}

		jobMeta.Statistics[metric] = schema.JobStatistics{
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
		return jobMeta, nil
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
	if err := json.NewEncoder(writer).Encode(jobMeta); err != nil {
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

	return jobMeta, f.Close()
}

// Add statisticsSeries fields
func calcStatisticsSeries(job *schema.Job, jobData schema.JobData) error {
	for _, scopes := range jobData {
		for _, jobMetric := range scopes {
			if jobMetric.StatisticsSeries != nil {
				continue
			}

			if len(jobMetric.Series) < 5 {
				continue
			}

			n := 0
			for _, series := range jobMetric.Series {
				if len(series.Data) > n {
					n = len(series.Data)
				}
			}

			mean, min, max := make([]schema.Float, n), make([]schema.Float, n), make([]schema.Float, n)
			for i := 0; i < n; i++ {
				sum, smin, smax := schema.Float(0.), math.MaxFloat32, -math.MaxFloat32
				for _, series := range jobMetric.Series {
					if i >= len(series.Data) {
						sum, smin, smax = schema.NaN, math.NaN(), math.NaN()
						break
					}
					x := series.Data[i]
					sum += x
					smin = math.Min(smin, float64(x))
					smax = math.Max(smax, float64(x))
				}
				sum /= schema.Float(len(jobMetric.Series))
				mean[i] = sum
				min[i] = schema.Float(smin)
				max[i] = schema.Float(smax)
			}

			jobMetric.StatisticsSeries = &schema.StatsSeries{
				Min: min, Mean: mean, Max: max,
			}
			jobMetric.Series = nil
		}
	}

	return nil
}
