package memorystore

import (
	"bufio"
	"fmt"
	"time"
)

// This is a threshold that allows a node to be healthy with certain number of data points missing.
// Suppose a node does not receive last 5 data points, then healthCheck endpoint will still say a
// node is healthy. Anything more than 5 missing points in metrics of the node will deem the node unhealthy.
const MaxMissingDataPoints int64 = 5

// This is a threshold which allows upto certain number of metrics in a node to be unhealthly.
// Works with MaxMissingDataPoints. Say 5 metrics (including submetrics) do not receive the last
// MaxMissingDataPoints data points, then the node will be deemed healthy. Any more metrics that does
// not receive data for MaxMissingDataPoints data points will deem the node unhealthy.
const MaxUnhealthyMetrics int64 = 5

func (b *buffer) healthCheck() int64 {

	// Check if the buffer is empty
	if b.data == nil {
		return 1
	}

	buffer_end := b.start + b.frequency*int64(len(b.data))
	t := time.Now().Unix()

	// Check if the buffer is too old
	if t-buffer_end > MaxMissingDataPoints*b.frequency {
		return 1
	}

	return 0
}

func (l *Level) healthCheck(m *MemoryStore, count int64) (int64, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	for _, mc := range m.Metrics {
		if b := l.metrics[mc.Offset]; b != nil {
			count += b.healthCheck()
		}
	}

	for _, lvl := range l.children {
		c, err := lvl.healthCheck(m, 0)
		if err != nil {
			return 0, err
		}
		count += c
	}

	return count, nil
}

func (m *MemoryStore) HealthCheck(w *bufio.Writer, selector []string) error {
	lvl := m.root.findLevel(selector)
	if lvl == nil {
		return fmt.Errorf("[METRICSTORE]> not found: %#v", selector)
	}

	buf := make([]byte, 0, 25)
	// buf = append(buf, "{"...)

	var count int64 = 0

	unhealthyMetricsCount, err := lvl.healthCheck(m, count)
	if err != nil {
		return err
	}

	if unhealthyMetricsCount < MaxUnhealthyMetrics {
		buf = append(buf, "Healthy"...)
	} else {
		buf = append(buf, "Unhealthy"...)
	}

	// buf = append(buf, "}\n"...)

	if _, err = w.Write(buf); err != nil {
		return err
	}

	return w.Flush()
}
