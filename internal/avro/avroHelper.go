package avro

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/config"
)

func DataStaging(wg *sync.WaitGroup, ctx context.Context) {

	// AvroPool is a pool of Avro writers.
	go func() {
		if config.MetricStoreKeys.Checkpoints.FileFormat == "json" {
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
				//Fetch the frequency of the metric from the global configuration
				freq, err := config.MetricStoreKeys.GetMetricFrequency(val.MetricName)
				if err != nil {
					fmt.Printf("Error fetching metric frequency: %s\n", err)
					continue
				}

				metricName := ""

				for _, selector_name := range val.Selector {
					metricName += selector_name + Delimiter
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
						fmt.Printf("Error creating or finding the level with cluster : %s, node : %s, metric : %s\n", val.Cluster, val.Node, val.MetricName)
					}
					oldSelector = append([]string{}, selector...)
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
