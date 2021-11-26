package metricdata

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/ClusterCockpit/cc-jobarchive/schema"
)

var runningJobs *CCMetricStore

func init() {
	runningJobs = &CCMetricStore{}
	if err := runningJobs.Init(); err != nil {
		log.Fatalln(err)
	}
}

// Fetches the metric data for a job.
func LoadData(job *model.Job, metrics []string, ctx context.Context) (schema.JobData, error) {
	if job.State == model.JobStateRunning {
		return runningJobs.LoadData(job, metrics, ctx)
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
