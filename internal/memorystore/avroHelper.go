// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package memorystore

import (
	"context"
	"slices"
	"strconv"
	"sync"

	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
)

func DataStaging(wg *sync.WaitGroup, ctx context.Context) {
	// AvroPool is a pool of Avro writers.
	go func() {
		if Keys.Checkpoints.FileFormat == "json" {
			wg.Done() // Mark this goroutine as done
			return    // Exit the goroutine
		}

		defer wg.Done()

		var avroLevel *AvroLevel
		oldSelector := make([]string, 0)

		for {
			select {
			case <-ctx.Done():
				return
			case val := <-LineProtocolMessages:
				// Fetch the frequency of the metric from the global configuration
				freq, err := GetMetricFrequency(val.MetricName)
				if err != nil {
					cclog.Errorf("Error fetching metric frequency: %s\n", err)
					continue
				}

				metricName := ""

				for _, selectorName := range val.Selector {
					metricName += selectorName + Delimiter
				}

				metricName += val.MetricName

				// Create a new selector for the Avro level
				// The selector is a slice of strings that represents the path to the
				// Avro level. It is created by appending the cluster, node, and metric
				// name to the selector.
				var selector []string
				selector = append(selector, val.Cluster, val.Node, strconv.FormatInt(freq, 10))

				if !testEq(oldSelector, selector) {
					// Get the Avro level for the metric
					avroLevel = avroStore.root.findAvroLevelOrCreate(selector)

					// If the Avro level is nil, create a new one
					if avroLevel == nil {
						cclog.Errorf("Error creating or finding the level with cluster : %s, node : %s, metric : %s\n", val.Cluster, val.Node, val.MetricName)
					}
					oldSelector = slices.Clone(selector)
				}

				avroLevel.addMetric(metricName, val.Value, val.Timestamp, int(freq))
			}
		}
	}()
}

func testEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
