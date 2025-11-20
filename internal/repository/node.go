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
			cache:     lrucache.New(repoConfig.CacheSize),
		}
	})
	return nodeRepoInstance
}

func (r *NodeRepository) FetchMetadata(hostname string, cluster string) (map[string]string, error) {
	start := time.Now()

	RawMetaData := make([]byte, 0)

	if err := sq.Select("node.meta_data").From("node").
		Where("node.hostname = ?", hostname).
		Where("node.cluster = ?", cluster).
		RunWith(r.stmtCache).QueryRow().Scan(&RawMetaData); err != nil {
		cclog.Warn("Error while scanning for node metadata")
		return nil, err
	}

	if len(RawMetaData) == 0 {
		return nil, nil
	}

	MetaData := make(map[string]string)

	if err := json.Unmarshal(RawMetaData, &MetaData); err != nil {
		cclog.Warn("Error while unmarshaling raw metadata json")
		return nil, err
	}

	cclog.Debugf("Timer FetchMetadata %s", time.Since(start))
	return MetaData, nil
}

func (r *NodeRepository) GetNode(hostname string, cluster string, withMeta bool) (*schema.Node, error) {
	node := &schema.Node{}
	var timestamp int
	if err := sq.Select("node.hostname", "node.cluster", "node.subcluster", "node_state.node_state",
		"node_state.health_state", "MAX(node_state.time_stamp) as time").
		From("node_state").
		Join("node ON node_state.node_id = node.id").
		Where("node.hostname = ?", hostname).
		Where("node.cluster = ?", cluster).
		GroupBy("node_state.node_id").
		RunWith(r.DB).
		QueryRow().Scan(&node.Hostname, &node.Cluster, &node.SubCluster, &node.NodeState, &node.HealthState, &timestamp); err != nil {
		cclog.Warnf("Error while querying node '%s' at time '%d' from database: %v", hostname, timestamp, err)
		return nil, err
	}

	if withMeta {
		var err error
		var meta map[string]string
		if meta, err = r.FetchMetadata(hostname, cluster); err != nil {
			cclog.Warnf("Error while fetching metadata for node '%s'", hostname)
			return nil, err
		}
		node.MetaData = meta
	}

	return node, nil
}

func (r *NodeRepository) GetNodeById(id int64, withMeta bool) (*schema.Node, error) {
	node := &schema.Node{}
	var timestamp int
	if err := sq.Select("node.hostname", "node.cluster", "node.subcluster", "node_state.node_state",
		"node_state.health_state", "MAX(node_state.time_stamp) as time").
		From("node_state").
		Join("node ON node_state.node_id = node.id").
		Where("node.id = ?", id).
		GroupBy("node_state.node_id").
		RunWith(r.DB).
		QueryRow().Scan(&node.Hostname, &node.Cluster, &node.SubCluster, &node.NodeState, &node.HealthState, &timestamp); err != nil {
		cclog.Warnf("Error while querying node ID '%d' at time '%d' from database: %v", id, timestamp, err)
		return nil, err
	}

	// NEEDS METADATA BY ID
	// if withMeta {
	// 	var err error
	// 	var meta map[string]string
	// 	if meta, err = r.FetchMetadata(hostname, cluster); err != nil {
	// 		cclog.Warnf("Error while fetching metadata for node '%s'", hostname)
	// 		return nil, err
	// 	}
	// 	node.MetaData = meta
	// }

	return node, nil
}

// const NamedNodeInsert string = `
// INSERT INTO node (time_stamp, hostname, cluster, subcluster, node_state, health_state,
//
//	cpus_allocated, cpus_total, memory_allocated, memory_total, gpus_allocated, gpus_total)
//	VALUES (:time_stamp, :hostname, :cluster, :subcluster, :node_state, :health_state,
//	:cpus_allocated, :cpus_total, :memory_allocated, :memory_total, :gpus_allocated, :gpus_total);`

const NamedNodeInsert string = `
INSERT INTO node (hostname, cluster, subcluster)
	VALUES (:hostname, :cluster, :subcluster);`

// AddNode adds a Node to the node table. This can be triggered by a node collector registration or
// from a nodestate update from the job scheduler.
func (r *NodeRepository) AddNode(node *schema.NodeDB) (int64, error) {
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

const NamedNodeStateInsert string = `
INSERT INTO node_state (time_stamp, node_state, health_state, cpus_allocated,
	memory_allocated, gpus_allocated, jobs_running, node_id)
	VALUES (:time_stamp, :node_state, :health_state, :cpus_allocated, :memory_allocated, :gpus_allocated, :jobs_running, :node_id);`

// TODO: Add real Monitoring Health State

// UpdateNodeState is called from the Node REST API to add a row in the node state table
func (r *NodeRepository) UpdateNodeState(hostname string, cluster string, nodeState *schema.NodeStateDB) error {
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
			node := schema.NodeDB{
				Hostname: hostname, Cluster: cluster, SubCluster: subcluster,
			}
			id, err = r.AddNode(&node)
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

	nodeState.NodeID = id

	_, err := r.DB.NamedExec(NamedNodeStateInsert, nodeState)
	if err != nil {
		cclog.Errorf("Error while adding node state for '%v' to database", hostname)
		return err
	}
	cclog.Infof("Updated node state for '%s' in database", hostname)
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

// QueryNodes returns a list of nodes based on a node filter. It always operates
// on the last state (largest timestamp).
func (r *NodeRepository) QueryNodes(
	ctx context.Context,
	filters []*model.NodeFilter,
	order *model.OrderByInput, // Currently unused!
) ([]*schema.Node, error) {
	query, qerr := AccessCheck(ctx,
		sq.Select("hostname", "cluster", "subcluster", "node_state",
			"health_state", "MAX(time_stamp) as time").
			From("node").
			Join("node_state ON node_state.node_id = node.id"))
	if qerr != nil {
		return nil, qerr
	}

	for _, f := range filters {
		if f.Hostname != nil {
			query = buildStringCondition("hostname", f.Hostname, query)
		}
		if f.Cluster != nil {
			query = buildStringCondition("cluster", f.Cluster, query)
		}
		if f.Subcluster != nil {
			query = buildStringCondition("subcluster", f.Subcluster, query)
		}
		if f.SchedulerState != nil {
			query = query.Where("node_state = ?", f.SchedulerState)
			// Requires Additional time_stamp Filter: Else the last (past!) time_stamp with queried state will be returned
			now := time.Now().Unix()
			query = query.Where(sq.Gt{"time_stamp": (now - 60)})
		}
		if f.HealthState != nil {
			query = query.Where("health_state = ?", f.HealthState)
			// Requires Additional time_stamp Filter: Else the last (past!) time_stamp with queried state will be returned
			now := time.Now().Unix()
			query = query.Where(sq.Gt{"time_stamp": (now - 60)})
		}
	}

	// Add Grouping and ORder after filters
	query = query.GroupBy("node_id").
		OrderBy("hostname ASC")

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := query.ToSql()
		cclog.Errorf("Error while running query '%s' %v: %v", queryString, queryVars, err)
		return nil, err
	}

	nodes := make([]*schema.Node, 0, 50)
	for rows.Next() {
		node := schema.Node{}
		var timestamp int
		if err := rows.Scan(&node.Hostname, &node.Cluster, &node.SubCluster,
			&node.NodeState, &node.HealthState, &timestamp); err != nil {
			rows.Close()
			cclog.Warnf("Error while scanning rows (QueryNodes) at time '%d'", timestamp)
			return nil, err
		}
		nodes = append(nodes, &node)
	}

	return nodes, nil
}

func (r *NodeRepository) ListNodes(cluster string) ([]*schema.Node, error) {
	q := sq.Select("node.hostname", "node.cluster", "node.subcluster", "node_state.node_state",
		"node_state.health_state", "MAX(node_state.time_stamp) as time").
		From("node").
		Join("node_state ON node_state.node_id = node.id").
		Where("node.cluster = ?", cluster).
		GroupBy("node_state.node_id").
		OrderBy("node.hostname ASC")

	rows, err := q.RunWith(r.DB).Query()
	if err != nil {
		cclog.Warn("Error while querying node list")
		return nil, err
	}
	nodeList := make([]*schema.Node, 0, 100)
	defer rows.Close()
	for rows.Next() {
		node := &schema.Node{}
		var timestamp int
		if err := rows.Scan(&node.Hostname, &node.Cluster,
			&node.SubCluster, &node.NodeState, &node.HealthState, &timestamp); err != nil {
			cclog.Warnf("Error while scanning node list (ListNodes) at time '%d'", timestamp)
			return nil, err
		}

		nodeList = append(nodeList, node)
	}

	return nodeList, nil
}

func (r *NodeRepository) MapNodes(cluster string) (map[string]string, error) {
	q := sq.Select("node.hostname", "node_state.node_state", "MAX(node_state.time_stamp) as time").
		From("node").
		Join("node_state ON node_state.node_id = node.id").
		Where("node.cluster = ?", cluster).
		GroupBy("node_state.node_id").
		OrderBy("node.hostname ASC")

	rows, err := q.RunWith(r.DB).Query()
	if err != nil {
		cclog.Warn("Error while querying node list")
		return nil, err
	}

	stateMap := make(map[string]string)
	defer rows.Close()
	for rows.Next() {
		var hostname, nodestate string
		var timestamp int
		if err := rows.Scan(&hostname, &nodestate, &timestamp); err != nil {
			cclog.Warnf("Error while scanning node list (MapNodes) at time '%d'", timestamp)
			return nil, err
		}

		stateMap[hostname] = nodestate
	}

	return stateMap, nil
}

func (r *NodeRepository) CountStates(ctx context.Context, filters []*model.NodeFilter, column string) ([]*model.NodeStates, error) {
	query, qerr := AccessCheck(ctx, sq.Select("hostname", column, "MAX(time_stamp) as time").From("node"))
	if qerr != nil {
		return nil, qerr
	}

	query = query.Join("node_state ON node_state.node_id = node.id")

	for _, f := range filters {
		if f.Hostname != nil {
			query = buildStringCondition("hostname", f.Hostname, query)
		}
		if f.Cluster != nil {
			query = buildStringCondition("cluster", f.Cluster, query)
		}
		if f.Subcluster != nil {
			query = buildStringCondition("subcluster", f.Subcluster, query)
		}
		if f.SchedulerState != nil {
			query = query.Where("node_state = ?", f.SchedulerState)
		}
		if f.HealthState != nil {
			query = query.Where("health_state = ?", f.HealthState)
		}
	}

	// Add Group and Order
	query = query.GroupBy("hostname").OrderBy("hostname DESC")

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := query.ToSql()
		cclog.Errorf("Error while running query '%s' %v: %v", queryString, queryVars, err)
		return nil, err
	}

	stateMap := map[string]int{}
	for rows.Next() {
		var hostname, state string
		var timestamp int

		if err := rows.Scan(&hostname, &state, &timestamp); err != nil {
			rows.Close()
			cclog.Warnf("Error while scanning rows (CountStates) at time '%d'", timestamp)
			return nil, err
		}

		stateMap[state] += 1
	}

	nodes := make([]*model.NodeStates, 0)
	for state, counts := range stateMap {
		node := model.NodeStates{State: state, Count: counts}
		nodes = append(nodes, &node)
	}

	return nodes, nil
}

func (r *NodeRepository) CountStatesTimed(ctx context.Context, filters []*model.NodeFilter, column string) ([]*model.NodeStatesTimed, error) {
	query, qerr := AccessCheck(ctx, sq.Select(column, "time_stamp", "count(*) as count").From("node")) // "cluster"?
	if qerr != nil {
		return nil, qerr
	}

	query = query.Join("node_state ON node_state.node_id = node.id")

	for _, f := range filters {
		// Required
		if f.TimeStart != nil {
			query = query.Where("time_stamp > ?", f.TimeStart)
		}
		// Optional
		if f.Hostname != nil {
			query = buildStringCondition("hostname", f.Hostname, query)
		}
		if f.Cluster != nil {
			query = buildStringCondition("cluster", f.Cluster, query)
		}
		if f.Subcluster != nil {
			query = buildStringCondition("subcluster", f.Subcluster, query)
		}
		if f.SchedulerState != nil {
			query = query.Where("node_state = ?", f.SchedulerState)
		}
		if f.HealthState != nil {
			query = query.Where("health_state = ?", f.HealthState)
		}
	}

	// Add Group and Order
	query = query.GroupBy(column + ", time_stamp").OrderBy("time_stamp ASC")

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := query.ToSql()
		cclog.Errorf("Error while running query '%s' %v: %v", queryString, queryVars, err)
		return nil, err
	}

	rawData := make(map[string][][]int)
	for rows.Next() {
		var state string
		var timestamp, count int

		if err := rows.Scan(&state, &timestamp, &count); err != nil {
			rows.Close()
			cclog.Warnf("Error while scanning rows (CountStatesTimed) at time '%d'", timestamp)
			return nil, err
		}

		if rawData[state] == nil {
			rawData[state] = [][]int{make([]int, 0), make([]int, 0)}
		}

		rawData[state][0] = append(rawData[state][0], timestamp)
		rawData[state][1] = append(rawData[state][1], count)
	}

	timedStates := make([]*model.NodeStatesTimed, 0)
	for state, data := range rawData {
		entry := model.NodeStatesTimed{State: state, Times: data[0], Counts: data[1]}
		timedStates = append(timedStates, &entry)
	}

	return timedStates, nil
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
