package graph

import (
	"context"
	"errors"

	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
)

func (r *queryResolver) JobMetricAverages(ctx context.Context, filter model.JobFilterList, metrics []*string) ([][]*float64, error) {
	return nil, errors.New("unimplemented")
}

func (r *queryResolver) RooflineHeatmap(ctx context.Context, filter model.JobFilterList, rows, cols int, minX, minY, maxX, maxY float64) ([][]float64, error) {
	return nil, errors.New("unimplemented")
}
