// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package graph

import (
	"context"

	"github.com/ClusterCockpit/cc-backend/internal/metricdispatch"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// Helpers for the Job footprint, energyFootprint and energy resolvers, placed
// here so that schema.resolvers.go is not too full. Running jobs (and stopped
// jobs not yet archived) get live values from the metric store; finished jobs
// get the values persisted at archiving. Live values are cached per job, so the
// three fields and the footprint query routing share one computation.

func (r *jobResolver) footprint(ctx context.Context, obj *schema.Job) (map[string]float64, error) {
	if lv, ok := metricdispatch.LiveFootprint(ctx, obj); ok {
		return lv.Footprint, nil
	}
	// scanJob already decoded the persisted footprint.
	if obj.Footprint != nil {
		return obj.Footprint, nil
	}
	return r.Repo.FetchFootprint(obj)
}

func (r *jobResolver) energyFootprint(ctx context.Context, obj *schema.Job) (map[string]float64, error) {
	if lv, ok := metricdispatch.LiveFootprint(ctx, obj); ok {
		return lv.EnergyFootprint, nil
	}
	return r.Repo.FetchEnergyFootprint(obj)
}

func (r *jobResolver) energy(ctx context.Context, obj *schema.Job) float64 {
	if lv, ok := metricdispatch.LiveFootprint(ctx, obj); ok {
		return lv.Energy
	}
	return obj.Energy
}
