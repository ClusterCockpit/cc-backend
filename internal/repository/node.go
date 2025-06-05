// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"encoding/json"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/lrucache"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

var (
	nodeRepoOnce     sync.Once
	nodeRepoInstance *NodeRepository
)

type NodeRepository struct {
	DB        *sqlx.DB
	stmtCache *sq.StmtCache
	cache     *lrucache.Cache
	driver    string
}

func GetNodeRepository() *NodeRepository {
	nodeRepoOnce.Do(func() {
		db := GetConnection()

		nodeRepoInstance = &NodeRepository{
			DB:     db.DB,
			driver: db.Driver,

			stmtCache: sq.NewStmtCache(db.DB),
			cache:     lrucache.New(1024 * 1024),
		}
	})
	return nodeRepoInstance
}

func (r *NodeRepository) FetchMetadata(node *schema.Node) (map[string]string, error) {
	start := time.Now()
	cachekey := fmt.Sprintf("metadata:%d", node.ID)
	if cached := r.cache.Get(cachekey, nil); cached != nil {
		node.MetaData = cached.(map[string]string)
		return node.MetaData, nil
	}

	if err := sq.Select("node.meta_data").From("node").Where("node.id = ?", node.ID).
		RunWith(r.stmtCache).QueryRow().Scan(&node.RawMetaData); err != nil {
		log.Warn("Error while scanning for node metadata")
		return nil, err
	}

	if len(node.RawMetaData) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(node.RawMetaData, &node.MetaData); err != nil {
		log.Warn("Error while unmarshaling raw metadata json")
		return nil, err
	}

	r.cache.Put(cachekey, node.MetaData, len(node.RawMetaData), 24*time.Hour)
	log.Debugf("Timer FetchMetadata %s", time.Since(start))
	return node.MetaData, nil
}

func (r *NodeRepository) UpdateMetadata(node *schema.Node, key, val string) (err error) {
	cachekey := fmt.Sprintf("metadata:%d", node.ID)
	r.cache.Del(cachekey)
	if node.MetaData == nil {
		if _, err = r.FetchMetadata(node); err != nil {
			log.Warnf("Error while fetching metadata for node, DB ID '%v'", node.ID)
			return err
		}
	}

	if node.MetaData != nil {
		cpy := make(map[string]string, len(node.MetaData)+1)
		maps.Copy(cpy, node.MetaData)
		cpy[key] = val
		node.MetaData = cpy
	} else {
		node.MetaData = map[string]string{key: val}
	}

	if node.RawMetaData, err = json.Marshal(node.MetaData); err != nil {
		log.Warnf("Error while marshaling metadata for node, DB ID '%v'", node.ID)
		return err
	}

	if _, err = sq.Update("node").
		Set("meta_data", node.RawMetaData).
		Where("node.id = ?", node.ID).
		RunWith(r.stmtCache).Exec(); err != nil {
		log.Warnf("Error while updating metadata for node, DB ID '%v'", node.ID)
		return err
	}

	r.cache.Put(cachekey, node.MetaData, len(node.RawMetaData), 24*time.Hour)
	return nil
}

func (r *NodeRepository) GetNode(id int64, withMeta bool) (*schema.Node, error) {
	node := &schema.Node{}
	if err := sq.Select("id", "hostname", "cluster", "subcluster", "node_state",
		"health_state").From("node").
		Where("node.id = ?", id).RunWith(r.DB).
		QueryRow().Scan(&node.ID, &node.Hostname, &node.Cluster, &node.SubCluster, &node.NodeState,
		&node.HealthState); err != nil {
		log.Warnf("Error while querying node '%v' from database", id)
		return nil, err
	}

	if withMeta {
		var err error
		var meta map[string]string
		if meta, err = r.FetchMetadata(node); err != nil {
			log.Warnf("Error while fetching metadata for node '%v'", id)
			return nil, err
		}
		node.MetaData = meta
	}

	return node, nil
}

const NamedNodeInsert string = `
INSERT INTO node (hostname, cluster, subcluster, node_state, health_state, raw_meta_data)
	VALUES (:hostname, :cluster, :subcluster, :node_state, :health_state, :raw_meta_data);`

func (r *NodeRepository) AddNode(node *schema.Node) (int64, error) {
	var err error
	node.RawMetaData, err = json.Marshal(node.MetaData)
	if err != nil {
		log.Errorf("Error while marshaling metadata for node '%v'", node.Hostname)
		return 0, err
	}

	res, err := r.DB.NamedExec(NamedNodeInsert, node)
	if err != nil {
		log.Errorf("Error while adding node '%v' to database", node.Hostname)
		return 0, err
	}
	node.ID, err = res.LastInsertId()
	if err != nil {
		log.Errorf("Error while getting last insert id for node '%v' from database", node.Hostname)
		return 0, err
	}

	return node.ID, nil
}

func (r *NodeRepository) UpdateNodeState(hostname string, nodeState *schema.NodeState) error {
	if _, err := sq.Update("node").Set("node_state", nodeState).Where("node.hostname = ?", hostname).RunWith(r.DB).Exec(); err != nil {
		log.Errorf("error while updating node '%s'", hostname)
		return err
	}

	return nil
}

func (r *NodeRepository) UpdateHealthState(hostname string, healthState *schema.MonitoringState) error {
	if _, err := sq.Update("node").Set("health_state", healthState).Where("node.id = ?", id).RunWith(r.DB).Exec(); err != nil {
		log.Errorf("error while updating node '%d'", id)
		return err
	}

	return nil
}

func (r *NodeRepository) DeleteNode(id int64) error {
	_, err := r.DB.Exec(`DELETE FROM node WHERE node.id = ?`, id)
	if err != nil {
		log.Errorf("Error while deleting node '%d' from DB", id)
		return err
	}
	log.Infof("deleted node '%d' from DB", id)
	return nil
}

func (r *NodeRepository) QueryNodes() ([]*schema.Node, error) {
	return nil, nil
}

func (r *NodeRepository) ListNodes(cluster string) ([]*schema.Node, error) {
	q := sq.Select("hostname", "cluster", "subcluster", "node_state",
		"health_state").From("node").Where("node.cluster = ?", cluster).OrderBy("node.hostname ASC")

	rows, err := q.RunWith(r.DB).Query()
	if err != nil {
		log.Warn("Error while querying user list")
		return nil, err
	}
	nodeList := make([]*schema.Node, 0, 100)
	defer rows.Close()
	for rows.Next() {
		node := &schema.Node{}
		if err := rows.Scan(&node.Hostname, &node.Cluster,
			&node.SubCluster, &node.NodeState, &node.HealthState); err != nil {
			log.Warn("Error while scanning node list")
			return nil, err
		}

		nodeList = append(nodeList, node)
	}

	return nodeList, nil
}
