package metricdata

import (
	"context"
	"errors"

	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/ClusterCockpit/cc-jobarchive/schema"
)

// Fetches the metric data for a job.
func LoadData(job *model.Job, metrics []string, ctx context.Context) (schema.JobData, error) {
	if job.State != model.JobStateCompleted {
		return nil, errors.New("only completed jobs are supported")
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
