// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"errors"
	"fmt"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

var (
	Clusters         []*schema.Cluster
	GlobalMetricList []*schema.GlobalMetricListItem
	nodeLists        map[string]map[string]NodeList
)

func initClusterConfig() error {
	Clusters = []*schema.Cluster{}
	nodeLists = map[string]map[string]NodeList{}
	metricLookup := make(map[string]schema.GlobalMetricListItem)

	for _, c := range ar.GetClusters() {

		cluster, err := ar.LoadClusterCfg(c)
		if err != nil {
			log.Warnf("Error while loading cluster config for cluster '%v'", c)
			return err
		}

		if len(cluster.Name) == 0 ||
			len(cluster.MetricConfig) == 0 ||
			len(cluster.SubClusters) == 0 {
			return errors.New("cluster.name, cluster.metricConfig and cluster.SubClusters should not be empty")
		}

		for _, mc := range cluster.MetricConfig {
			if len(mc.Name) == 0 {
				return errors.New("cluster.metricConfig.name should not be empty")
			}
			if mc.Timestep < 1 {
				return errors.New("cluster.metricConfig.timestep should not be smaller than one")
			}

			// For backwards compability...
			if mc.Scope == "" {
				mc.Scope = schema.MetricScopeNode
			}
			if !mc.Scope.Valid() {
				return errors.New("cluster.metricConfig.scope must be a valid scope ('node', 'scocket', ...)")
			}

			ml, ok := metricLookup[mc.Name]
			if !ok {
				metricLookup[mc.Name] = schema.GlobalMetricListItem{
					Name: mc.Name, Scope: mc.Scope, Unit: mc.Unit, Footprint: mc.Footprint,
				}
				ml = metricLookup[mc.Name]
			}
			availability := schema.ClusterSupport{Cluster: cluster.Name}
			scLookup := make(map[string]*schema.SubClusterConfig)

			for _, scc := range mc.SubClusters {
				scLookup[scc.Name] = scc
			}

			for _, sc := range cluster.SubClusters {
				newMetric := mc
				newMetric.SubClusters = nil

				if cfg, ok := scLookup[sc.Name]; ok {
					if !cfg.Remove {
						availability.SubClusters = append(availability.SubClusters, sc.Name)
						newMetric.Peak = cfg.Peak
						newMetric.Peak = cfg.Peak
						newMetric.Normal = cfg.Normal
						newMetric.Caution = cfg.Caution
						newMetric.Alert = cfg.Alert
						newMetric.Footprint = cfg.Footprint
						newMetric.Energy = cfg.Energy
						newMetric.LowerIsBetter = cfg.LowerIsBetter
						sc.MetricConfig = append(sc.MetricConfig, *newMetric)

						if newMetric.Footprint {
							sc.Footprint = append(sc.Footprint, newMetric.Name)
							ml.Footprint = true
						}
						if newMetric.Energy {
							sc.EnergyFootprint = append(sc.EnergyFootprint, newMetric.Name)
						}
					}
				} else {
					availability.SubClusters = append(availability.SubClusters, sc.Name)
					sc.MetricConfig = append(sc.MetricConfig, *newMetric)

					if newMetric.Footprint {
						sc.Footprint = append(sc.Footprint, newMetric.Name)
					}
					if newMetric.Energy {
						sc.EnergyFootprint = append(sc.EnergyFootprint, newMetric.Name)
					}
				}
			}
			ml.Availability = append(metricLookup[mc.Name].Availability, availability)
			metricLookup[mc.Name] = ml
		}

		Clusters = append(Clusters, cluster)

		nodeLists[cluster.Name] = make(map[string]NodeList)
		for _, sc := range cluster.SubClusters {
			if sc.Nodes == "*" {
				continue
			}

			nl, err := ParseNodeList(sc.Nodes)
			if err != nil {
				return fmt.Errorf("ARCHIVE/CLUSTERCONFIG > in %s/cluster.json: %w", cluster.Name, err)
			}
			nodeLists[cluster.Name][sc.Name] = nl
		}
	}

	for _, ml := range metricLookup {
		GlobalMetricList = append(GlobalMetricList, &ml)
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
func AssignSubCluster(job *schema.BaseJob) error {
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
	for sc, nl := range nodeLists[job.Cluster] {
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
	for sc, nl := range nodeLists[cluster] {
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
