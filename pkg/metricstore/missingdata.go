// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"fmt"
	"maps"
	"slices"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

// gapInfo summarises the buffers of one or more reads that do not cover the
// requested window. Missing points are counted at the metric's frequency.
type gapInfo struct {
	buffers  int // buffers missing points at the front or the back
	maxFront int // most points missing before a buffer's first sample
	maxBack  int // most points missing after a buffer's last sample
}

// record counts one buffer missing front points before and back points after
// its data.
func (g *gapInfo) record(front, back int) {
	g.buffers++
	g.maxFront = max(g.maxFront, front)
	g.maxBack = max(g.maxBack, back)
}

// merge adds the buffers of o to g.
func (g *gapInfo) merge(o gapInfo) {
	g.buffers += o.buffers
	g.maxFront = max(g.maxFront, o.maxFront)
	g.maxBack = max(g.maxBack, o.maxBack)
}

// hostGaps accumulates the gaps of all reads for one host.
type hostGaps struct {
	metrics map[string]struct{}
	gapInfo
}

// missingReport collects missing-data gaps per host for one FetchData or
// FetchStats call, so that each affected host is logged once per request
// instead of once per buffer. The zero value is ready to use and allocates
// nothing until a gap is added.
type missingReport struct {
	hosts map[string]*hostGaps
}

// add merges the gaps of one read of metric on host. Reads without gaps are
// ignored.
func (r *missingReport) add(host, metric string, g gapInfo) {
	if g.buffers == 0 {
		return
	}
	if r.hosts == nil {
		r.hosts = make(map[string]*hostGaps)
	}
	h, ok := r.hosts[host]
	if !ok {
		h = &hostGaps{metrics: make(map[string]struct{})}
		r.hosts[host] = h
	}
	h.metrics[metric] = struct{}{}
	h.merge(g)
}

// lines formats one summary line per host, hosts sorted.
func (r *missingReport) lines(cluster string, from, to int64) []string {
	if len(r.hosts) == 0 {
		return nil
	}
	out := make([]string, 0, len(r.hosts))
	for _, host := range slices.Sorted(maps.Keys(r.hosts)) {
		h := r.hosts[host]
		out = append(out, fmt.Sprintf(
			"[METRICSTORE]> missing data points: node=%s/%s requested[%d,%d] metrics=%d max_missing_front=%d pts max_missing_back=%d pts",
			cluster, host, from, to, len(h.metrics), h.maxFront, h.maxBack))
	}
	return out
}

// log writes the summary lines as warnings.
func (r *missingReport) log(cluster string, from, to int64) {
	for _, line := range r.lines(cluster, from, to) {
		cclog.Warn(line)
	}
}
