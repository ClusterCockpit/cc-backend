package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/auth"
	"github.com/ClusterCockpit/cc-backend/graph/model"
	"github.com/ClusterCockpit/cc-backend/schema"
	"github.com/iamlouk/lrucache"
	"github.com/jmoiron/sqlx"
)

var db *sqlx.DB
var lookupConfigStmt *sqlx.Stmt

var lock sync.RWMutex
var uiDefaults map[string]interface{}

var cache *lrucache.Cache = lrucache.New(1024)

var Clusters []*model.Cluster
var nodeLists map[string]map[string]NodeList

func Init(usersdb *sqlx.DB, authEnabled bool, uiConfig map[string]interface{}, jobArchive string) error {
	db = usersdb
	uiDefaults = uiConfig
	entries, err := os.ReadDir(jobArchive)
	if err != nil {
		return err
	}

	Clusters = []*model.Cluster{}
	nodeLists = map[string]map[string]NodeList{}
	for _, de := range entries {
		raw, err := os.ReadFile(filepath.Join(jobArchive, de.Name(), "cluster.json"))
		if err != nil {
			return err
		}

		var cluster model.Cluster

		// Disabled because of the historic 'measurement' field.
		// dec := json.NewDecoder(bytes.NewBuffer(raw))
		// dec.DisallowUnknownFields()
		// if err := dec.Decode(&cluster); err != nil {
		// 	return err
		// }

		if err := json.Unmarshal(raw, &cluster); err != nil {
			return err
		}

		if len(cluster.Name) == 0 || len(cluster.MetricConfig) == 0 || len(cluster.SubClusters) == 0 {
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

		if cluster.FilterRanges.StartTime.To.IsZero() {
			cluster.FilterRanges.StartTime.To = time.Unix(0, 0)
		}

		if cluster.Name != de.Name() {
			return fmt.Errorf("the file '.../%s/cluster.json' contains the clusterId '%s'", de.Name(), cluster.Name)
		}

		Clusters = append(Clusters, &cluster)

		nodeLists[cluster.Name] = make(map[string]NodeList)
		for _, sc := range cluster.SubClusters {
			if sc.Nodes == "" {
				continue
			}

			nl, err := ParseNodeList(sc.Nodes)
			if err != nil {
				return fmt.Errorf("in %s/cluster.json: %w", cluster.Name, err)
			}
			nodeLists[cluster.Name][sc.Name] = nl
		}
	}

	if authEnabled {
		_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS configuration (
			username varchar(255),
			confkey  varchar(255),
			value    varchar(255),
			PRIMARY KEY (username, confkey),
			FOREIGN KEY (username) REFERENCES user (username) ON DELETE CASCADE ON UPDATE NO ACTION);`)
		if err != nil {
			return err
		}

		lookupConfigStmt, err = db.Preparex(`SELECT confkey, value FROM configuration WHERE configuration.username = ?`)
		if err != nil {
			return err
		}
	}

	return nil
}

// Return the personalised UI config for the currently authenticated
// user or return the plain default config.
func GetUIConfig(r *http.Request) (map[string]interface{}, error) {
	user := auth.GetUser(r.Context())
	if user == nil {
		lock.RLock()
		copy := make(map[string]interface{}, len(uiDefaults))
		for k, v := range uiDefaults {
			copy[k] = v
		}
		lock.RUnlock()
		return copy, nil
	}

	data := cache.Get(user.Username, func() (interface{}, time.Duration, int) {
		config := make(map[string]interface{}, len(uiDefaults))
		for k, v := range uiDefaults {
			config[k] = v
		}

		rows, err := lookupConfigStmt.Query(user.Username)
		if err != nil {
			return err, 0, 0
		}

		size := 0
		defer rows.Close()
		for rows.Next() {
			var key, rawval string
			if err := rows.Scan(&key, &rawval); err != nil {
				return err, 0, 0
			}

			var val interface{}
			if err := json.Unmarshal([]byte(rawval), &val); err != nil {
				return err, 0, 0
			}

			size += len(key)
			size += len(rawval)
			config[key] = val
		}

		return config, 24 * time.Hour, size
	})
	if err, ok := data.(error); ok {
		return nil, err
	}

	return data.(map[string]interface{}), nil
}

// If the context does not have a user, update the global ui configuration without persisting it!
// If there is a (authenticated) user, update only his configuration.
func UpdateConfig(key, value string, ctx context.Context) error {
	user := auth.GetUser(ctx)
	if user == nil {
		var val interface{}
		if err := json.Unmarshal([]byte(value), &val); err != nil {
			return err
		}

		lock.Lock()
		defer lock.Unlock()
		uiDefaults[key] = val
		return nil
	}

	// Disabled because now `plot_list_selectedMetrics:<cluster>` is possible.
	// if _, ok := uiDefaults[key]; !ok {
	// 	return errors.New("this configuration key does not exist")
	// }

	if _, err := db.Exec(`REPLACE INTO configuration (username, confkey, value) VALUES (?, ?, ?)`,
		user.Username, key, value); err != nil {
		return err
	}

	cache.Del(user.Username)
	return nil
}

func GetCluster(cluster string) *model.Cluster {
	for _, c := range Clusters {
		if c.Name == cluster {
			return c
		}
	}
	return nil
}

func GetSubCluster(cluster, subcluster string) *model.SubCluster {
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

func GetMetricConfig(cluster, metric string) *model.MetricConfig {
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
