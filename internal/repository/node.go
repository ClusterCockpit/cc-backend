// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/lrucache"
	"github.com/ClusterCockpit/cc-lib/schema"
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

var nodeColumns []string = []string{
	// "node.id,"
	"node.hostname", "node.cluster", "node.subcluster",
	"node.node_state", "node.health_state", // "node.meta_data",
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
		cclog.Warn("Error while scanning for node metadata")
		return nil, err
	}

	if len(node.RawMetaData) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(node.RawMetaData, &node.MetaData); err != nil {
		cclog.Warn("Error while unmarshaling raw metadata json")
		return nil, err
	}

	r.cache.Put(cachekey, node.MetaData, len(node.RawMetaData), 24*time.Hour)
	cclog.Debugf("Timer FetchMetadata %s", time.Since(start))
	return node.MetaData, nil
}

func (r *NodeRepository) UpdateMetadata(node *schema.Node, key, val string) (err error) {
	cachekey := fmt.Sprintf("metadata:%d", node.ID)
	r.cache.Del(cachekey)
	if node.MetaData == nil {
		if _, err = r.FetchMetadata(node); err != nil {
			cclog.Warnf("Error while fetching metadata for node, DB ID '%v'", node.ID)
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
		cclog.Warnf("Error while marshaling metadata for node, DB ID '%v'", node.ID)
		return err
	}

	if _, err = sq.Update("node").
		Set("meta_data", node.RawMetaData).
		Where("node.id = ?", node.ID).
		RunWith(r.stmtCache).Exec(); err != nil {
		cclog.Warnf("Error while updating metadata for node, DB ID '%v'", node.ID)
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
		cclog.Warnf("Error while querying node '%v' from database", id)
		return nil, err
	}

	if withMeta {
		var err error
		var meta map[string]string
		if meta, err = r.FetchMetadata(node); err != nil {
			cclog.Warnf("Error while fetching metadata for node '%v'", id)
			return nil, err
		}
		node.MetaData = meta
	}

	return node, nil
}

const NamedNodeInsert string = `
INSERT INTO node (hostname, cluster, subcluster, node_state, health_state)
	VALUES (:hostname, :cluster, :subcluster, :node_state, :health_state);`

func (r *NodeRepository) AddNode(node *schema.Node) (int64, error) {
	var err error

	res, err := r.DB.NamedExec(NamedNodeInsert, node)
	if err != nil {
		cclog.Errorf("Error while adding node '%v' to database", node.Hostname)
		return 0, err
	}
	node.ID, err = res.LastInsertId()
	if err != nil {
		cclog.Errorf("Error while getting last insert id for node '%v' from database", node.Hostname)
		return 0, err
	}

	return node.ID, nil
}

func (r *NodeRepository) UpdateNodeState(hostname string, cluster string, nodeState *schema.NodeState) error {
	var id int64
	if err := sq.Select("id").From("node").
		Where("node.hostname = ?", hostname).Where("node.cluster = ?", cluster).RunWith(r.DB).
		QueryRow().Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			subcluster, err := archive.GetSubClusterByNode(cluster, hostname)
			if err != nil {
				cclog.Errorf("Error while getting subcluster for node '%s' in cluster '%s': %v", hostname, cluster, err)
				return err
			}
			node := schema.Node{
				Hostname: hostname, Cluster: cluster, SubCluster: subcluster, NodeState: *nodeState,
				HealthState: schema.MonitoringStateFull,
			}
			_, err = r.AddNode(&node)
			if err != nil {
				cclog.Errorf("Error while adding node '%s' to database: %v", hostname, err)
				return err
			}

			cclog.Infof("Added node '%s' to database", hostname)
			return nil
		} else {
			cclog.Warnf("Error while querying node '%v' from database", id)
			return err
		}
	}

	if _, err := sq.Update("node").Set("node_state", nodeState).Where("node.id = ?", id).RunWith(r.DB).Exec(); err != nil {
		cclog.Errorf("error while updating node '%s'", hostname)
		return err
	}
	cclog.Infof("Updated node '%s' in database", hostname)
	return nil
}

// func (r *NodeRepository) UpdateHealthState(hostname string, healthState *schema.MonitoringState) error {
// 	if _, err := sq.Update("node").Set("health_state", healthState).Where("node.id = ?", id).RunWith(r.DB).Exec(); err != nil {
// 		cclog.Errorf("error while updating node '%d'", id)
// 		return err
// 	}
//
// 	return nil
// }

func (r *NodeRepository) DeleteNode(id int64) error {
	_, err := r.DB.Exec(`DELETE FROM node WHERE node.id = ?`, id)
	if err != nil {
		cclog.Errorf("Error while deleting node '%d' from DB", id)
		return err
	}
	cclog.Infof("deleted node '%d' from DB", id)
	return nil
}

// TODO: Implement order by
func (r *NodeRepository) QueryNodes(
	ctx context.Context,
	filters []*model.NodeFilter,
	order *model.OrderByInput, // Currently unused!
) ([]*schema.Node, error) {
	query, qerr := AccessCheck(ctx, sq.Select(nodeColumns...).From("node"))
	if qerr != nil {
		return nil, qerr
	}

	for _, f := range filters {
		if f.Hostname != nil {
			query = buildStringCondition("node.hostname", f.Hostname, query)
		}
		if f.Cluster != nil {
			query = buildStringCondition("node.cluster", f.Cluster, query)
		}
		if f.Subcluster != nil {
			query = buildStringCondition("node.subcluster", f.Subcluster, query)
		}
		if f.NodeState != nil {
			query = query.Where("node.node_state = ?", f.NodeState)
		}
		if f.HealthState != nil {
			query = query.Where("node.health_state = ?", f.HealthState)
		}
	}

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := query.ToSql()
		cclog.Errorf("Error while running query '%s' %v: %v", queryString, queryVars, err)
		return nil, err
	}

	nodes := make([]*schema.Node, 0, 50)
	for rows.Next() {
		node := schema.Node{}

		if err := rows.Scan(&node.Hostname, &node.Cluster, &node.SubCluster,
			&node.NodeState, &node.HealthState); err != nil {
			rows.Close()
			cclog.Warn("Error while scanning rows (Nodes)")
			return nil, err
		}
		nodes = append(nodes, &node)
	}

	return nodes, nil
}

func (r *NodeRepository) ListNodes(cluster string) ([]*schema.Node, error) {
	q := sq.Select("hostname", "cluster", "subcluster", "node_state",
		"health_state").From("node").Where("node.cluster = ?", cluster).OrderBy("node.hostname ASC")

	rows, err := q.RunWith(r.DB).Query()
	if err != nil {
		cclog.Warn("Error while querying user list")
		return nil, err
	}
	nodeList := make([]*schema.Node, 0, 100)
	defer rows.Close()
	for rows.Next() {
		node := &schema.Node{}
		if err := rows.Scan(&node.Hostname, &node.Cluster,
			&node.SubCluster, &node.NodeState, &node.HealthState); err != nil {
			cclog.Warn("Error while scanning node list")
			return nil, err
		}

		nodeList = append(nodeList, node)
	}

	return nodeList, nil
}

func (r *NodeRepository) CountNodeStates(ctx context.Context, filters []*model.NodeFilter) ([]*model.NodeStates, error) {
	query, qerr := AccessCheck(ctx, sq.Select("node_state AS state", "count(*) AS count").From("node"))
	if qerr != nil {
		return nil, qerr
	}

	for _, f := range filters {
		if f.Hostname != nil {
			query = buildStringCondition("node.hostname", f.Hostname, query)
		}
		if f.Cluster != nil {
			query = buildStringCondition("node.cluster", f.Cluster, query)
		}
		if f.Subcluster != nil {
			query = buildStringCondition("node.subcluster", f.Subcluster, query)
		}
		if f.NodeState != nil {
			query = query.Where("node.node_state = ?", f.NodeState)
		}
		if f.HealthState != nil {
			query = query.Where("node.health_state = ?", f.HealthState)
		}
	}

	// Add Group and Order
	query = query.GroupBy("state").OrderBy("count DESC")

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := query.ToSql()
		cclog.Errorf("Error while running query '%s' %v: %v", queryString, queryVars, err)
		return nil, err
	}

	nodes := make([]*model.NodeStates, 0)
	for rows.Next() {
		node := model.NodeStates{}

		if err := rows.Scan(&node.State, &node.Count); err != nil {
			rows.Close()
			cclog.Warn("Error while scanning rows (NodeStates)")
			return nil, err
		}
		nodes = append(nodes, &node)
	}

	return nodes, nil
}

func (r *NodeRepository) CountHealthStates(ctx context.Context, filters []*model.NodeFilter) ([]*model.NodeStates, error) {
	query, qerr := AccessCheck(ctx, sq.Select("health_state AS state", "count(*) AS count").From("node"))
	if qerr != nil {
		return nil, qerr
	}

	for _, f := range filters {
		if f.Hostname != nil {
			query = buildStringCondition("node.hostname", f.Hostname, query)
		}
		if f.Cluster != nil {
			query = buildStringCondition("node.cluster", f.Cluster, query)
		}
		if f.Subcluster != nil {
			query = buildStringCondition("node.subcluster", f.Subcluster, query)
		}
		if f.NodeState != nil {
			query = query.Where("node.node_state = ?", f.NodeState)
		}
		if f.HealthState != nil {
			query = query.Where("node.health_state = ?", f.HealthState)
		}
	}

	// Add Group and Order
	query = query.GroupBy("state").OrderBy("count DESC")

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := query.ToSql()
		cclog.Errorf("Error while running query '%s' %v: %v", queryString, queryVars, err)
		return nil, err
	}

	nodes := make([]*model.NodeStates, 0)
	for rows.Next() {
		node := model.NodeStates{}

		if err := rows.Scan(&node.State, &node.Count); err != nil {
			rows.Close()
			cclog.Warn("Error while scanning rows (NodeStates)")
			return nil, err
		}
		nodes = append(nodes, &node)
	}

	return nodes, nil
}

func AccessCheck(ctx context.Context, query sq.SelectBuilder) (sq.SelectBuilder, error) {
	user := GetUserFromContext(ctx)
	return AccessCheckWithUser(user, query)
}

func AccessCheckWithUser(user *schema.User, query sq.SelectBuilder) (sq.SelectBuilder, error) {
	if user == nil {
		var qnil sq.SelectBuilder
		return qnil, fmt.Errorf("user context is nil")
	}

	switch {
	// case len(user.Roles) == 1 && user.HasRole(schema.RoleApi): // API-User : Access NodeInfos
	// 	return query, nil
	case user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport}): // Admin & Support : Access NodeInfos
		return query, nil
	default: // No known Role: No Access, return error
		var qnil sq.SelectBuilder
		return qnil, fmt.Errorf("user has no or unknown roles")
	}
}
