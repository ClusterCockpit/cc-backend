// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archive

import (
	"fmt"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

var (
	Clusters             []*schema.Cluster
	GlobalMetricList     []*schema.GlobalMetricListItem
	GlobalUserMetricList []*schema.GlobalMetricListItem
	NodeLists            map[string]map[string]NodeList
)

func initClusterConfig() error {
	Clusters = []*schema.Cluster{}
	GlobalMetricList = []*schema.GlobalMetricListItem{}
	GlobalUserMetricList = []*schema.GlobalMetricListItem{}
	NodeLists = map[string]map[string]NodeList{}
	metricLookup := make(map[string]schema.GlobalMetricListItem)

	for _, c := range ar.GetClusters() {

		cluster, err := ar.LoadClusterCfg(c)
		if err != nil {
			cclog.Warnf("Error while loading cluster config for cluster '%v'", c)
			return fmt.Errorf("failed to load cluster config for '%s': %w", c, err)
		}

		if len(cluster.Name) == 0 {
			return fmt.Errorf("cluster name is empty in config for '%s'", c)
		}
		if len(cluster.MetricConfig) == 0 {
			return fmt.Errorf("cluster '%s' has no metric configurations", cluster.Name)
		}
		if len(cluster.SubClusters) == 0 {
			return fmt.Errorf("cluster '%s' has no subclusters defined", cluster.Name)
		}

		for _, mc := range cluster.MetricConfig {
			if len(mc.Name) == 0 {
				return fmt.Errorf("cluster '%s' has a metric config with empty name", cluster.Name)
			}
			if mc.Timestep < 1 {
				return fmt.Errorf("metric '%s' in cluster '%s' has invalid timestep %d (must be >= 1)", mc.Name, cluster.Name, mc.Timestep)
			}

			// For backwards compatibility...
			if mc.Scope == "" {
				mc.Scope = schema.MetricScopeNode
			}
			if !mc.Scope.Valid() {
				return fmt.Errorf("metric '%s' in cluster '%s' has invalid scope '%s' (must be 'node', 'socket', 'core', etc.)", mc.Name, cluster.Name, mc.Scope)
			}

			if _, ok := metricLookup[mc.Name]; !ok {
				metricLookup[mc.Name] = schema.GlobalMetricListItem{
					Name: mc.Name, Scope: mc.Scope, Restrict: mc.Restrict, Unit: mc.Unit, Footprint: mc.Footprint,
				}
			}

			availability := schema.ClusterSupport{Cluster: cluster.Name}
			scLookup := make(map[string]*schema.SubClusterConfig)

			for _, scc := range mc.SubClusters {
				scLookup[scc.Name] = scc
			}

			for _, sc := range cluster.SubClusters {
				newMetric := &schema.MetricConfig{
					Metric: schema.Metric{
						Name:    mc.Name,
						Unit:    mc.Unit,
						Peak:    mc.Peak,
						Normal:  mc.Normal,
						Caution: mc.Caution,
						Alert:   mc.Alert,
					},
					Energy:        mc.Energy,
					Scope:         mc.Scope,
					Aggregation:   mc.Aggregation,
					Timestep:      mc.Timestep,
					LowerIsBetter: mc.LowerIsBetter,
				}

				if mc.Footprint != "" {
					newMetric.Footprint = mc.Footprint
				}

				if cfg, ok := scLookup[sc.Name]; ok {
					if cfg.Remove {
						continue
					}
					newMetric.Peak = cfg.Peak
					newMetric.Normal = cfg.Normal
					newMetric.Caution = cfg.Caution
					newMetric.Alert = cfg.Alert
					newMetric.Footprint = cfg.Footprint
					newMetric.Energy = cfg.Energy
					newMetric.LowerIsBetter = cfg.LowerIsBetter
				}

				availability.SubClusters = append(availability.SubClusters, sc.Name)
				sc.MetricConfig = append(sc.MetricConfig, newMetric)

				if newMetric.Footprint != "" {
					sc.Footprint = append(sc.Footprint, newMetric.Name)
					item := metricLookup[mc.Name]
					item.Footprint = newMetric.Footprint
					metricLookup[mc.Name] = item
				}
				if newMetric.Energy != "" {
					sc.EnergyFootprint = append(sc.EnergyFootprint, newMetric.Name)
				}
			}

			item := metricLookup[mc.Name]
			item.Availability = append(item.Availability, availability)
			metricLookup[mc.Name] = item
		}

		Clusters = append(Clusters, cluster)

		NodeLists[cluster.Name] = make(map[string]NodeList)
		for _, sc := range cluster.SubClusters {
			if sc.Nodes == "*" {
				continue
			}

			nl, err := ParseNodeList(sc.Nodes)
			if err != nil {
				return fmt.Errorf("ARCHIVE/CLUSTERCONFIG > in %s/cluster.json: %w", cluster.Name, err)
			}
			NodeLists[cluster.Name][sc.Name] = nl
		}
	}

	for _, metric := range metricLookup {
		GlobalMetricList = append(GlobalMetricList, &metric)
		if !metric.Restrict {
			GlobalUserMetricList = append(GlobalUserMetricList, &metric)
		}
	}

	return nil
}

func GetCluster(cluster string) *schema.Cluster {
	for _, c := range Clusters {
		if c.Name == cluster {
			return c
		}
	}
	return nil
}

func GetSubCluster(cluster, subcluster string) (*schema.SubCluster, error) {
	for _, c := range Clusters {
		if c.Name == cluster {
			for _, p := range c.SubClusters {
				if p.Name == subcluster {
					return p, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("subcluster '%v' not found for cluster '%v', or cluster '%v' not configured", subcluster, cluster, cluster)
}

func GetMetricConfigSubCluster(cluster, subcluster string) map[string]*schema.Metric {
	metrics := make(map[string]*schema.Metric)

	for _, c := range Clusters {
		if c.Name == cluster {
			for _, m := range c.MetricConfig {
				for _, s := range m.SubClusters {
					if s.Name == subcluster {
						metrics[m.Name] = &schema.Metric{
							Name:    m.Name,
							Unit:    s.Unit,
							Peak:    s.Peak,
							Normal:  s.Normal,
							Caution: s.Caution,
							Alert:   s.Alert,
						}
						break
					}
				}

				_, ok := metrics[m.Name]
				if !ok {
					metrics[m.Name] = &schema.Metric{
						Name:    m.Name,
						Unit:    m.Unit,
						Peak:    m.Peak,
						Normal:  m.Normal,
						Caution: m.Caution,
						Alert:   m.Alert,
					}
				}
			}
			break
		}
	}

	return metrics
}

func GetMetricConfig(cluster, metric string) *schema.MetricConfig {
	for _, c := range Clusters {
		if c.Name == cluster {
			for _, m := range c.MetricConfig {
				if m.Name == metric {
					return m
				}
			}
		}
	}
	return nil
}

// AssignSubCluster sets the `job.subcluster` property of the job based
// on its cluster and resources.
func AssignSubCluster(job *schema.Job) error {
	cluster := GetCluster(job.Cluster)
	if cluster == nil {
		return fmt.Errorf("ARCHIVE/CLUSTERCONFIG > unkown cluster: %v", job.Cluster)
	}

	if job.SubCluster != "" {
		for _, sc := range cluster.SubClusters {
			if sc.Name == job.SubCluster {
				return nil
			}
		}
		return fmt.Errorf("ARCHIVE/CLUSTERCONFIG > already assigned subcluster %v unkown (cluster: %v)", job.SubCluster, job.Cluster)
	}

	if len(job.Resources) == 0 {
		return fmt.Errorf("ARCHIVE/CLUSTERCONFIG > job without any resources/hosts")
	}

	host0 := job.Resources[0].Hostname
	for sc, nl := range NodeLists[job.Cluster] {
		if nl != nil && nl.Contains(host0) {
			job.SubCluster = sc
			return nil
		}
	}

	if cluster.SubClusters[0].Nodes == "*" {
		job.SubCluster = cluster.SubClusters[0].Name
		return nil
	}

	return fmt.Errorf("ARCHIVE/CLUSTERCONFIG > no subcluster found for cluster %v and host %v", job.Cluster, host0)
}

func GetSubClusterByNode(cluster, hostname string) (string, error) {
	for sc, nl := range NodeLists[cluster] {
		if nl != nil && nl.Contains(hostname) {
			return sc, nil
		}
	}

	c := GetCluster(cluster)
	if c == nil {
		return "", fmt.Errorf("ARCHIVE/CLUSTERCONFIG > unkown cluster: %v", cluster)
	}

	if c.SubClusters[0].Nodes == "" {
		return c.SubClusters[0].Name, nil
	}

	return "", fmt.Errorf("ARCHIVE/CLUSTERCONFIG > no subcluster found for cluster %v and host %v", cluster, hostname)
}

func MetricIndex(mc []*schema.MetricConfig, name string) (int, error) {
	for i, m := range mc {
		if m.Name == name {
			return i, nil
		}
	}

	return 0, fmt.Errorf("unknown metric name %s", name)
}
