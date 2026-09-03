// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricdispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	ccms "github.com/ClusterCockpit/cc-backend/internal/metricstoreclient"
	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
)

type MetricDataRepository interface {
	// Return the JobData for the given job, only with the requested metrics.
	LoadData(job *schema.Job,
		metrics []string,
		scopes []schema.MetricScope,
		ctx context.Context,
		resolution int,
		resampleAlgo string) (schema.JobData, error)

	// Return a map of metrics to a map of nodes to the metric statistics of the job. node scope only.
	LoadStats(job *schema.Job,
		metrics []string,
		ctx context.Context) (map[string]map[string]schema.MetricStatistics, error)

	// Return a map of metrics to a map of scopes to the scoped metric statistics of the job.
	LoadScopedStats(job *schema.Job,
		metrics []string,
		scopes []schema.MetricScope,
		ctx context.Context) (schema.ScopedJobStats, error)

	// Return a map of hosts to a map of metrics at the requested scopes (currently only node) for that node.
	LoadNodeData(cluster string,
		metrics, nodes []string,
		scopes []schema.MetricScope,
		from, to time.Time,
		ctx context.Context) (map[string]map[string][]*schema.JobMetric, error)

	// Return a map of hosts to a map of metrics to a map of scopes for multiple nodes.
	LoadNodeListData(cluster, subCluster string,
		nodes []string,
		metrics []string,
		scopes []schema.MetricScope,
		resolution int,
		from, to time.Time,
		ctx context.Context,
		resampleAlgo string) (map[string]schema.JobData, error)

	// HealthCheck evaluates the monitoring state for a set of nodes against expected metrics.
	HealthCheck(cluster string,
		nodes []string,
		metrics []string) (map[string]metricstore.HealthCheckResult, error)
}

type CCMetricStoreConfig struct {
	Scope string `json:"scope"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

var metricDataRepos map[string]MetricDataRepository = map[string]MetricDataRepository{}

// metricStoreTokenEnv returns the scope-specific environment variable name for
// an external metric store's token, or "" when the scope has none.
//
// The metric-store-external section is an array with one entry per scope, so a
// single fixed name cannot address one particular entry. The scope, which is a
// stable identifier the operator already chose, is appended instead: scope
// "fritz-spr1tb" becomes METRICSTORE_TOKEN_FRITZ_SPR1TB.
//
// The wildcard scope "*" has no scope-specific name and uses the generic one.
// A scope that sanitizes to "FILE" is also rejected, because the resulting
// name would collide with the generic name's own "_FILE" variant.
func metricStoreTokenEnv(scope string) string {
	if scope == "" || scope == "*" {
		return ""
	}

	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r >= 'a' && r <= 'z':
			return r - ('a' - 'A')
		default:
			return '_'
		}
	}, scope)

	if sanitized == "FILE" {
		cclog.Warnf("[METRICDISPATCH]> scope %q would collide with %s%s; ignoring its scope-specific environment variable",
			scope, config.EnvMetricStoreToken, util.EnvFileSuffix)
		return ""
	}

	return config.EnvMetricStoreToken + "_" + sanitized
}

// resolveStoreToken resolves one metric store's token. The scope-specific
// environment variable wins over the generic one, which wins over the
// configured value.
func resolveStoreToken(cfg CCMetricStoreConfig) (string, error) {
	token, err := util.SecretFromEnv(config.EnvMetricStoreToken, cfg.Token)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", config.EnvMetricStoreToken, err)
	}

	if name := metricStoreTokenEnv(cfg.Scope); name != "" {
		if token, err = util.SecretFromEnv(name, token); err != nil {
			return "", fmt.Errorf("resolving %s: %w", name, err)
		}
	}

	return token, nil
}

func Init(rawConfig json.RawMessage) error {
	if rawConfig != nil {
		var configs []CCMetricStoreConfig
		config.Validate(configSchema, rawConfig)
		dec := json.NewDecoder(bytes.NewReader(rawConfig))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&configs); err != nil {
			// The raw config is deliberately not included here: it carries the
			// store tokens, and this error is logged by the caller.
			return fmt.Errorf("[METRICDISPATCH]> External Metric Store Config Init: Could not decode config: %s", err.Error())
		}

		if len(configs) == 0 {
			return fmt.Errorf("[METRICDISPATCH]> No external metric store configurations found in config file")
		}

		for _, storeConfig := range configs {
			token, err := resolveStoreToken(storeConfig)
			if err != nil {
				return fmt.Errorf("[METRICDISPATCH]> External Metric Store Config Init: %w", err)
			}
			if token == "" {
				cclog.Warnf("[METRICDISPATCH]> no token for metric store scope %q: requests will be unauthenticated",
					storeConfig.Scope)
			}
			metricDataRepos[storeConfig.Scope] = ccms.NewCCMetricStore(storeConfig.URL, token)
		}
	}

	return nil
}

func GetMetricDataRepo(cluster string, subcluster string) (MetricDataRepository, error) {
	var repo MetricDataRepository
	var ok bool

	key := cluster + "-" + subcluster
	repo, ok = metricDataRepos[key]

	if !ok {
		repo, ok = metricDataRepos[cluster]

		if !ok {
			repo, ok = metricDataRepos["*"]

			if !ok {
				if metricstore.MetricStoreHandle == nil {
					return nil, fmt.Errorf("[METRICDISPATCH]> no metric data repository configured '%s'", key)
				}

				repo = metricstore.MetricStoreHandle
				cclog.Debugf("[METRICDISPATCH]> Using internal metric data repository for '%s'", key)
			}
		}
	}

	return repo, nil
}

// GetHealthCheckRepo returns the MetricDataRepository for performing health checks on a cluster.
// It uses the same fallback logic as GetMetricDataRepo: cluster → wildcard → internal.
func GetHealthCheckRepo(cluster string) (MetricDataRepository, error) {
	return GetMetricDataRepo(cluster, "")
}
