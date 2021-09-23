package graph

import (
	"context"
	"errors"
	"time"

	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
)

func (r *queryResolver) UserStats(ctx context.Context, from *time.Time, to *time.Time, clusterId *string) ([]*model.UserStats, error) {
	return nil, errors.New("unimplemented")
}
