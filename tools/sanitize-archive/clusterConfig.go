// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"errors"
	"fmt"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

var Clusters []*Cluster
var nodeLists map[string]map[string]archive.NodeList

func initClusterConfig() error {

	Clusters = []*Cluster{}
	nodeLists = map[string]map[string]archive.NodeList{}

	for _, c := range ar.GetClusters() {

		cluster, err := ar.LoadClusterCfg(c)
		if err != nil {
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
		}

		Clusters = append(Clusters, cluster)

		nodeLists[cluster.Name] = make(map[string]archive.NodeList)
		for _, sc := range cluster.SubClusters {
			if sc.Nodes == "" {
				continue
			}

			nl, err := archive.ParseNodeList(sc.Nodes)
			if err != nil {
				return fmt.Errorf("in %s/cluster.json: %w", cluster.Name, err)
			}
			nodeLists[cluster.Name][sc.Name] = nl
		}
	}

	return nil
}

func GetCluster(cluster string) *Cluster {

	for _, c := range Clusters {
		if c.Name == cluster {
			return c
		}
	}
	return nil
}

func GetSubCluster(cluster, subcluster string) *SubCluster {

	for _, c := range Clusters {
		if c.Name == cluster {
			for _, p := range c.SubClusters {
				if p.Name == subcluster {
					return p
				}
			}
		}
	}
	return nil
}

func GetMetricConfig(cluster, metric string) *MetricConfig {

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
func AssignSubCluster(job *BaseJob) error {

	cluster := GetCluster(job.Cluster)
	if cluster == nil {
		return fmt.Errorf("unkown cluster: %#v", job.Cluster)
	}

	if job.SubCluster != "" {
		for _, sc := range cluster.SubClusters {
			if sc.Name == job.SubCluster {
				return nil
			}
		}
		return fmt.Errorf("already assigned subcluster %#v unkown (cluster: %#v)", job.SubCluster, job.Cluster)
	}

	if len(job.Resources) == 0 {
		return fmt.Errorf("job without any resources/hosts")
	}

	host0 := job.Resources[0].Hostname
	for sc, nl := range nodeLists[job.Cluster] {
		if nl != nil && nl.Contains(host0) {
			job.SubCluster = sc
			return nil
		}
	}

	if cluster.SubClusters[0].Nodes == "" {
		job.SubCluster = cluster.SubClusters[0].Name
		return nil
	}

	return fmt.Errorf("no subcluster found for cluster %#v and host %#v", job.Cluster, host0)
}

func GetSubClusterByNode(cluster, hostname string) (string, error) {

	for sc, nl := range nodeLists[cluster] {
		if nl != nil && nl.Contains(hostname) {
			return sc, nil
		}
	}

	c := GetCluster(cluster)
	if c == nil {
		return "", fmt.Errorf("unkown cluster: %#v", cluster)
	}

	if c.SubClusters[0].Nodes == "" {
		return c.SubClusters[0].Name, nil
	}

	return "", fmt.Errorf("no subcluster found for cluster %#v and host %#v", cluster, hostname)
}
