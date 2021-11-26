package config

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
)

var lock sync.RWMutex
var config map[string]interface{}

var Clusters []*model.Cluster

const configFilePath string = "./var/ui.config.json"

func init() {
	lock.Lock()
	defer lock.Unlock()

	bytes, err := os.ReadFile(configFilePath)
	if err != nil {
		log.Fatal(err)
	}

	if err := json.Unmarshal(bytes, &config); err != nil {
		log.Fatal(err)
	}
}

// Call this function to change the current configuration.
// `value` must be valid JSON. This This function is thread-safe.
func UpdateConfig(key, value string, ctx context.Context) error {
	var v interface{}
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		return err
	}

	lock.Lock()
	defer lock.Unlock()

	config[key] = v
	bytes, err := json.Marshal(config)
	if err != nil {
		return err
	}

	if err := os.WriteFile(configFilePath, bytes, 0644); err != nil {
		return err
	}

	return nil
}

// http.HandlerFunc compatible function that serves the current configuration as JSON
func ServeConfig(rw http.ResponseWriter, r *http.Request) {
	lock.RLock()
	defer lock.RUnlock()

	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(config); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}
}

func GetClusterConfig(cluster string) *model.Cluster {
	for _, c := range Clusters {
		if c.ClusterID == cluster {
			return c
		}
	}
	return nil
}

func GetMetricConfig(cluster, metric string) *model.MetricConfig {
	for _, c := range Clusters {
		if c.ClusterID == cluster {
			for _, m := range c.MetricConfig {
				if m.Name == metric {
					return m
				}
			}
		}
	}
	return nil
}
