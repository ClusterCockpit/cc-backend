package metricdata

import (
	"context"
	"errors"
	"fmt"

	"github.com/ClusterCockpit/cc-jobarchive/config"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/ClusterCockpit/cc-jobarchive/schema"
)

type MetricDataRepository interface {
	Init(url string) error
	LoadData(job *model.Job, metrics []string, ctx context.Context) (schema.JobData, error)
}

var metricDataRepos map[string]MetricDataRepository = map[string]MetricDataRepository{}

var JobArchivePath string

func Init(jobArchivePath string) error {
	JobArchivePath = jobArchivePath
	for _, cluster := range config.Clusters {
		if cluster.MetricDataRepository != nil {
			switch cluster.MetricDataRepository.Kind {
			case "cc-metric-store":
				ccms := &CCMetricStore{}
				if err := ccms.Init(cluster.MetricDataRepository.Url); err != nil {
					return err
				}
				metricDataRepos[cluster.ClusterID] = ccms
			case "influxdb-v2":
				idb := &InfluxDBv2DataRepository{}
				if err := idb.Init(cluster.MetricDataRepository.Url); err != nil {
					return err
				}
				metricDataRepos[cluster.ClusterID] = idb
			default:
				return fmt.Errorf("unkown metric data repository '%s' for cluster '%s'", cluster.MetricDataRepository.Kind, cluster.ClusterID)
			}
		}
	}
	return nil
}

// Fetches the metric data for a job.
func LoadData(job *model.Job, metrics []string, ctx context.Context) (schema.JobData, error) {
	if job.State == model.JobStateRunning {
		repo, ok := metricDataRepos[job.ClusterID]
		if !ok {
			return nil, fmt.Errorf("no metric data repository configured for '%s'", job.ClusterID)
		}

		return repo.LoadData(job, metrics, ctx)
	}

	if job.State != model.JobStateCompleted {
		return nil, fmt.Errorf("job of state '%s' is not supported", job.State)
	}

	data, err := loadFromArchive(job)
	if err != nil {
		return nil, err
	}

	if metrics != nil {
		res := schema.JobData{}
		for _, metric := range metrics {
			if metricdata, ok := data[metric]; ok {
				res[metric] = metricdata
			}
		}
		return res, nil
	}
	return data, nil
}

// Used for the jobsFootprint GraphQL-Query. TODO: Rename/Generalize.
func LoadAverages(job *model.Job, metrics []string, data [][]schema.Float, ctx context.Context) error {
	if job.State != model.JobStateCompleted {
		return errors.New("only completed jobs are supported")
	}

	return loadAveragesFromArchive(job, metrics, data)
}
