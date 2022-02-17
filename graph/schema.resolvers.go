package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ClusterCockpit/cc-backend/auth"
	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/graph/generated"
	"github.com/ClusterCockpit/cc-backend/graph/model"
	"github.com/ClusterCockpit/cc-backend/metricdata"
	"github.com/ClusterCockpit/cc-backend/schema"
)

func (r *jobResolver) Tags(ctx context.Context, obj *schema.Job) ([]*schema.Tag, error) {
	return r.Repo.GetTags(&obj.ID)
}

func (r *mutationResolver) CreateTag(ctx context.Context, typeArg string, name string) (*schema.Tag, error) {
	id, err := r.Repo.CreateTag(typeArg, name)
	if err != nil {
		return nil, err
	}

	return &schema.Tag{ID: id, Type: typeArg, Name: name}, nil
}

func (r *mutationResolver) DeleteTag(ctx context.Context, id string) (string, error) {
	// The UI does not allow this currently anyways.
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) AddTagsToJob(ctx context.Context, job string, tagIds []string) ([]*schema.Tag, error) {
	jid, err := strconv.ParseInt(job, 10, 64)
	if err != nil {
		return nil, err
	}

	for _, tagId := range tagIds {
		tid, err := strconv.ParseInt(tagId, 10, 64)
		if err != nil {
			return nil, err
		}

		if err := r.Repo.AddTag(jid, tid); err != nil {
			return nil, err
		}
	}

	j, err := r.Query().Job(ctx, job)
	if err != nil {
		return nil, err
	}

	j.Tags, err = r.Repo.GetTags(&jid)
	if err != nil {
		return nil, err
	}

	return j.Tags, metricdata.UpdateTags(j, j.Tags)
}

func (r *mutationResolver) RemoveTagsFromJob(ctx context.Context, job string, tagIds []string) ([]*schema.Tag, error) {
	jid, err := strconv.ParseInt(job, 10, 64)
	if err != nil {
		return nil, err
	}

	for _, tagId := range tagIds {
		tid, err := strconv.ParseInt(tagId, 10, 64)
		if err != nil {
			return nil, err
		}

		if err := r.Repo.RemoveTag(jid, tid); err != nil {
			return nil, err
		}
	}

	dummyJob := schema.Job{}
	dummyJob.ID = int64(jid)
	tags, err := r.Job().Tags(ctx, &dummyJob)
	if err != nil {
		return nil, err
	}

	jobObj, err := r.Query().Job(ctx, job)
	if err != nil {
		return nil, err
	}

	return tags, metricdata.UpdateTags(jobObj, tags)
}

func (r *mutationResolver) UpdateConfiguration(ctx context.Context, name string, value string) (*string, error) {
	if err := config.UpdateConfig(name, value, ctx); err != nil {
		return nil, err
	}

	return nil, nil
}

func (r *queryResolver) Clusters(ctx context.Context) ([]*model.Cluster, error) {
	return config.Clusters, nil
}

func (r *queryResolver) Tags(ctx context.Context) ([]*schema.Tag, error) {
	return r.Repo.GetTags(nil)
}

func (r *queryResolver) Job(ctx context.Context, id string) (*schema.Job, error) {
	numericId, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, err
	}

	job, err := r.Repo.FindById(numericId)
	if err != nil {
		return nil, err
	}

	if user := auth.GetUser(ctx); user != nil && !user.HasRole(auth.RoleAdmin) && job.User != user.Username {
		return nil, errors.New("you are not allowed to see this job")
	}

	return job, nil
}

func (r *queryResolver) JobMetrics(ctx context.Context, id string, metrics []string, scopes []schema.MetricScope) ([]*model.JobMetricWithName, error) {
	job, err := r.Query().Job(ctx, id)
	if err != nil {
		return nil, err
	}

	data, err := metricdata.LoadData(job, metrics, scopes, ctx)
	if err != nil {
		return nil, err
	}

	res := []*model.JobMetricWithName{}
	for name, md := range data {
		for scope, metric := range md {
			if metric.Scope != schema.MetricScope(scope) {
				panic("WTF?")
			}

			res = append(res, &model.JobMetricWithName{
				Name:   name,
				Metric: metric,
			})
		}
	}

	return res, err
}

func (r *queryResolver) JobsFootprints(ctx context.Context, filter []*model.JobFilter, metrics []string) ([]*model.MetricFootprints, error) {
	return r.jobsFootprints(ctx, filter, metrics)
}

func (r *queryResolver) Jobs(ctx context.Context, filter []*model.JobFilter, page *model.PageRequest, order *model.OrderByInput) (*model.JobResultList, error) {
	jobs, err := r.Repo.QueryJobs(ctx, filter, page, order)
	if err != nil {
		return nil, err
	}

	count, err := r.Repo.CountJobs(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &model.JobResultList{Items: jobs, Count: &count}, nil
}

func (r *queryResolver) JobsStatistics(ctx context.Context, filter []*model.JobFilter, groupBy *model.Aggregate) ([]*model.JobsStatistics, error) {
	return r.jobsStatistics(ctx, filter, groupBy)
}

func (r *queryResolver) RooflineHeatmap(ctx context.Context, filter []*model.JobFilter, rows int, cols int, minX float64, minY float64, maxX float64, maxY float64) ([][]float64, error) {
	return r.rooflineHeatmap(ctx, filter, rows, cols, minX, minY, maxX, maxY)
}

func (r *queryResolver) NodeMetrics(ctx context.Context, cluster string, partition *string, nodes []string, scopes []schema.MetricScope, metrics []string, from time.Time, to time.Time) ([]*model.NodeMetrics, error) {
	user := auth.GetUser(ctx)
	if user != nil && !user.HasRole(auth.RoleAdmin) {
		return nil, errors.New("you need to be an administrator for this query")
	}

	if partition == nil {
		partition = new(string)
	}

	if metrics == nil {
		for _, mc := range config.GetClusterConfig(cluster).MetricConfig {
			metrics = append(metrics, mc.Name)
		}
	}

	data, err := metricdata.LoadNodeData(cluster, *partition, metrics, nodes, scopes, from, to, ctx)
	if err != nil {
		return nil, err
	}

	nodeMetrics := make([]*model.NodeMetrics, 0, len(data))
	for hostname, metrics := range data {
		host := &model.NodeMetrics{
			Host:    hostname,
			Metrics: make([]*model.JobMetricWithName, 0, len(metrics)*len(scopes)),
		}

		for metric, scopedMetrics := range metrics {
			for _, scopedMetric := range scopedMetrics {
				host.Metrics = append(host.Metrics, &model.JobMetricWithName{
					Name:   metric,
					Metric: scopedMetric,
				})
			}
		}

		nodeMetrics = append(nodeMetrics, host)
	}

	return nodeMetrics, nil
}

// Job returns generated.JobResolver implementation.
func (r *Resolver) Job() generated.JobResolver { return &jobResolver{r} }

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type jobResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
