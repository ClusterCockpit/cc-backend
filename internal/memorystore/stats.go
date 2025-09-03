package memorystore

import (
	"errors"
	"math"

	"github.com/ClusterCockpit/cc-lib/util"
)

type Stats struct {
	Samples int
	Avg     util.Float
	Min     util.Float
	Max     util.Float
}

func (b *buffer) stats(from, to int64) (Stats, int64, int64, error) {
	if from < b.start {
		if b.prev != nil {
			return b.prev.stats(from, to)
		}
		from = b.start
	}

	// TODO: Check if b.closed and if so and the full buffer is queried,
	// use b.statistics instead of iterating over the buffer.

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
			idx = 0
		}

		if t < b.start || idx >= len(b.data) {
			continue
		}

		xf := float64(b.data[idx])
		if math.IsNaN(xf) {
			continue
		}

		samples += 1
		sum += xf
		min = math.Min(min, xf)
		max = math.Max(max, xf)
	}

	return Stats{
		Samples: samples,
		Avg:     util.Float(sum) / util.Float(samples),
		Min:     util.Float(min),
		Max:     util.Float(max),
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
		return nil, 0, 0, errors.New("unkown metric: " + metric)
	}

	n, samples := 0, 0
	avg, min, max := util.Float(0), math.MaxFloat32, -math.MaxFloat32
	err := m.root.findBuffers(selector, minfo.Offset, func(b *buffer) error {
		stats, cfrom, cto, err := b.stats(from, to)
		if err != nil {
			return err
		}

		if n == 0 {
			from, to = cfrom, cto
		} else if from != cfrom || to != cto {
			return ErrDataDoesNotAlign
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
		avg /= util.Float(n)
	} else if n > 1 && minfo.Aggregation != SumAggregation {
		return nil, 0, 0, errors.New("invalid aggregation")
	}

	return &Stats{
		Samples: samples,
		Avg:     avg,
		Min:     util.Float(min),
		Max:     util.Float(max),
	}, from, to, nil
}
