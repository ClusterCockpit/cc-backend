package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ClusterCockpit/cc-jobarchive/config"
	"github.com/ClusterCockpit/cc-jobarchive/graph/generated"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/ClusterCockpit/cc-jobarchive/metricdata"
	sq "github.com/Masterminds/squirrel"
)

func (r *jobResolver) Tags(ctx context.Context, obj *model.Job) ([]*model.JobTag, error) {
	query := sq.
		Select("tag.id", "tag.tag_type", "tag.tag_name").
		From("tag").
		Join("jobtag ON jobtag.tag_id = tag.id").
		Where("jobtag.job_id = ?", obj.ID)

	rows, err := query.RunWith(r.DB).Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tags := make([]*model.JobTag, 0)
	for rows.Next() {
		var tag model.JobTag
		if err := rows.Scan(&tag.ID, &tag.TagType, &tag.TagName); err != nil {
			return nil, err
		}
		tags = append(tags, &tag)
	}

	return tags, nil
}

func (r *mutationResolver) CreateTag(ctx context.Context, typeArg string, name string) (*model.JobTag, error) {
	res, err := r.DB.Exec("INSERT INTO tag (tag_type, tag_name) VALUES ($1, $2)", typeArg, name)
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &model.JobTag{ID: strconv.FormatInt(id, 10), TagType: typeArg, TagName: name}, nil
}

func (r *mutationResolver) DeleteTag(ctx context.Context, id string) (string, error) {
	// The UI does not allow this currently anyways.
	panic(fmt.Errorf("not implemented"))
}

func (r *mutationResolver) AddTagsToJob(ctx context.Context, job string, tagIds []string) ([]*model.JobTag, error) {
	jid, err := strconv.Atoi(job)
	if err != nil {
		return nil, err
	}

	for _, tagId := range tagIds {
		tid, err := strconv.Atoi(tagId)
		if err != nil {
			return nil, err
		}

		if _, err := r.DB.Exec("INSERT INTO jobtag (job_id, tag_id) VALUES ($1, $2)", jid, tid); err != nil {
			return nil, err
		}
	}

	tags, err := r.Job().Tags(ctx, &model.Job{ID: job})
	if err != nil {
		return nil, err
	}

	jobObj, err := r.Query().Job(ctx, job)
	if err != nil {
		return nil, err
	}

	return tags, metricdata.UpdateTags(jobObj, tags)
}

func (r *mutationResolver) RemoveTagsFromJob(ctx context.Context, job string, tagIds []string) ([]*model.JobTag, error) {
	jid, err := strconv.Atoi(job)
	if err != nil {
		return nil, err
	}

	for _, tagId := range tagIds {
		tid, err := strconv.Atoi(tagId)
		if err != nil {
			return nil, err
		}

		if _, err := r.DB.Exec("DELETE FROM jobtag WHERE jobtag.job_id = $1 AND jobtag.tag_id = $2", jid, tid); err != nil {
			return nil, err
		}
	}

	tags, err := r.Job().Tags(ctx, &model.Job{ID: job})
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

func (r *queryResolver) Tags(ctx context.Context) ([]*model.JobTag, error) {
	rows, err := sq.Select("id", "tag_type", "tag_name").From("tag").RunWith(r.DB).Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tags := make([]*model.JobTag, 0)
	for rows.Next() {
		var tag model.JobTag
		if err := rows.Scan(&tag.ID, &tag.TagType, &tag.TagName); err != nil {
			return nil, err
		}
		tags = append(tags, &tag)
	}

	return tags, nil
}

func (r *queryResolver) Job(ctx context.Context, id string) (*model.Job, error) {
	query := sq.Select(JobTableCols...).From("job").Where("job.id = ?", id)
	query = securityCheck(ctx, query)
	return ScanJob(query.RunWith(r.DB).QueryRow())
}

func (r *queryResolver) JobMetrics(ctx context.Context, id string, metrics []string) ([]*model.JobMetricWithName, error) {
	job, err := r.Query().Job(ctx, id)
	if err != nil {
		return nil, err
	}

	data, err := metricdata.LoadData(job, metrics, ctx)
	if err != nil {
		return nil, err
	}

	res := []*model.JobMetricWithName{}
	for name, md := range data {
		res = append(res, &model.JobMetricWithName{
			Name:   name,
			Metric: md,
		})
	}

	return res, err
}

func (r *queryResolver) JobsFootprints(ctx context.Context, filter []*model.JobFilter, metrics []string) ([]*model.MetricFootprints, error) {
	return r.jobsFootprints(ctx, filter, metrics)
}

func (r *queryResolver) Jobs(ctx context.Context, filter []*model.JobFilter, page *model.PageRequest, order *model.OrderByInput) (*model.JobResultList, error) {
	jobs, count, err := r.queryJobs(ctx, filter, page, order)
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

// Job returns generated.JobResolver implementation.
func (r *Resolver) Job() generated.JobResolver { return &jobResolver{r} }

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type jobResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
