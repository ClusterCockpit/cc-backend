// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package metricstore

import (
	"errors"
	"math"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
)

type Stats struct {
	Samples int
	Avg     schema.Float
	Min     schema.Float
	Max     schema.Float
}

// recomputeStats rebuilds the buffer's running statistics from a single full
// scan of its data and marks them valid. Used after an overwrite invalidated
// the incremental aggregate, and at checkpoint load time.
func (b *buffer) recomputeStats() {
	sum, samples := 0.0, 0
	min, max := math.MaxFloat32, -math.MaxFloat32
	for _, v := range b.data {
		xf := float64(v)
		if math.IsNaN(xf) {
			continue
		}
		samples++
		sum += xf
		if xf < min {
			min = xf
		}
		if xf > max {
			max = xf
		}
	}
	b.statSum = sum
	b.statSamples = samples
	b.statMin = min
	b.statMax = max
	b.statsValid = true
}

func (b *buffer) stats(from, to int64) (Stats, int64, int64, error) {
	if from < b.start {
		if b.prev != nil {
			return b.prev.stats(from, to)
		}
		from = b.start
	}

	samples := 0
	sum, min, max := 0.0, math.MaxFloat32, -math.MaxFloat32

	var t int64
	for t = from; t < to; t += b.frequency {
		idx := int((t - b.start) / b.frequency)
		if idx >= cap(b.data) {
			b = b.next
			if b == nil {
				break
			}
			idx = int((t - b.start) / b.frequency)
		}

		// Fast path: standing at this buffer's first data point (idx 0) with the
		// whole buffer inside [from, to). Fold in the cached aggregate and jump
		// past the buffer's real data instead of scanning each point. Any slots
		// between len(data) and cap are handled as gaps by the normal loop after
		// t advances, so the returned `to` matches the scan semantics.
		if len(b.data) > 0 && idx <= 0 && t <= b.firstWrite() && b.end() <= to {
			if !b.statsValid {
				b.recomputeStats()
			}
			if b.statSamples > 0 {
				sum += b.statSum
				samples += b.statSamples
				if b.statMin < min {
					min = b.statMin
				}
				if b.statMax > max {
					max = b.statMax
				}
			}
			// Position t at the buffer's last real data point; the loop's
			// t += frequency then advances into the trailing gap / next buffer.
			t = b.end() - b.frequency
			continue
		}

		if t < b.start || idx >= len(b.data) {
			continue
		}

		xf := float64(b.data[idx])
		if math.IsNaN(xf) {
			continue
		}

		samples++
		sum += xf
		if xf < min {
			min = xf
		}
		if xf > max {
			max = xf
		}
	}

	return Stats{
		Samples: samples,
		Avg:     schema.Float(sum) / schema.Float(samples),
		Min:     schema.Float(min),
		Max:     schema.Float(max),
	}, from, t, nil
}

// Returns statistics for the requested metric on the selected node/level.
// Data is aggregated to the selected level the same way as in `MemoryStore.Read`.
// If `Stats.Samples` is zero, the statistics should not be considered as valid.
func (m *MemoryStore) Stats(selector util.Selector, metric string, from, to int64) (*Stats, int64, int64, error) {
	if from > to {
		return nil, 0, 0, errors.New("invalid time range")
	}

	minfo, ok := m.Metrics[metric]
	if !ok {
		return nil, 0, 0, errors.New("unknown metric: " + metric)
	}

	n, samples := 0, 0
	avg, min, max := schema.Float(0), math.MaxFloat32, -math.MaxFloat32
	err := m.root.findBuffers(selector, minfo.offset, func(b *buffer) error {
		stats, cfrom, cto, err := b.stats(from, to)
		if err != nil {
			return err
		}

		if n == 0 {
			from, to = cfrom, cto
		} else if from != cfrom {
			return ErrDataDoesNotAlignMissingFront
		} else if to != cto {
			return ErrDataDoesNotAlignMissingBack
		}

		samples += stats.Samples
		avg += stats.Avg
		min = math.Min(min, float64(stats.Min))
		max = math.Max(max, float64(stats.Max))
		n += 1
		return nil
	})
	if err != nil {
		return nil, 0, 0, err
	}

	if n == 0 {
		return nil, 0, 0, ErrNoData
	}

	if minfo.Aggregation == AvgAggregation {
		avg /= schema.Float(n)
	} else if n > 1 && minfo.Aggregation != SumAggregation {
		return nil, 0, 0, errors.New("invalid aggregation")
	}

	return &Stats{
		Samples: samples,
		Avg:     avg,
		Min:     schema.Float(min),
		Max:     schema.Float(max),
	}, from, to, nil
}
