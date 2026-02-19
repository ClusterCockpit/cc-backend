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
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/lrucache"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
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

// latestStateCondition returns a squirrel expression that restricts node_state
// rows to the latest per node_id using a correlated subquery.
// Requires the query to join node and node_state tables.
func latestStateCondition() sq.Sqlizer {
	return sq.Expr(
		"node_state.id = (SELECT ns2.id FROM node_state ns2 WHERE ns2.node_id = node.id ORDER BY ns2.time_stamp DESC LIMIT 1)",
	)
}

// applyNodeFilters applies common NodeFilter conditions to a query that joins
// the node and node_state tables with latestStateCondition.
func applyNodeFilters(query sq.SelectBuilder, filters []*model.NodeFilter) sq.SelectBuilder {
	for _, f := range filters {
		if f.Cluster != nil {
			query = buildStringCondition("node.cluster", f.Cluster, query)
		}
		if f.SubCluster != nil {
			query = buildStringCondition("node.subcluster", f.SubCluster, query)
		}
		if f.Hostname != nil {
			query = buildStringCondition("node.hostname", f.Hostname, query)
		}
		if f.SchedulerState != nil {
			query = query.Where("node_state.node_state = ?", f.SchedulerState)
		}
		if f.HealthState != nil {
			query = query.Where("node_state.health_state = ?", f.HealthState)
		}
	}
	return query
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
	if err := sq.Select("node.hostname", "node.cluster", "node.subcluster",
		"node_state.node_state", "node_state.health_state").
		From("node").
		Join("node_state ON node_state.node_id = node.id").
		Where(latestStateCondition()).
		Where("node.hostname = ?", hostname).
		Where("node.cluster = ?", cluster).
		RunWith(r.DB).
		QueryRow().Scan(&node.Hostname, &node.Cluster, &node.SubCluster, &node.NodeState, &node.HealthState); err != nil {
		cclog.Warnf("Error while querying node '%s' from database: %v", hostname, err)
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

func (r *NodeRepository) GetNodeByID(id int64, withMeta bool) (*schema.Node, error) {
	node := &schema.Node{}
	if err := sq.Select("node.hostname", "node.cluster", "node.subcluster",
		"node_state.node_state", "node_state.health_state").
		From("node").
		Join("node_state ON node_state.node_id = node.id").
		Where(latestStateCondition()).
		Where("node.id = ?", id).
		RunWith(r.DB).
		QueryRow().Scan(&node.Hostname, &node.Cluster, &node.SubCluster, &node.NodeState, &node.HealthState); err != nil {
		cclog.Warnf("Error while querying node ID '%d' from database: %v", id, err)
		return nil, err
	}

	if withMeta {
		meta, metaErr := r.FetchMetadata(node.Hostname, node.Cluster)
		if metaErr != nil {
			cclog.Warnf("Error while fetching metadata for node ID '%d': %v", id, metaErr)
			return nil, metaErr
		}
		node.MetaData = meta
	}

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
INSERT INTO node_state (time_stamp, node_state, health_state, health_metrics,
	cpus_allocated, memory_allocated, gpus_allocated, jobs_running, node_id)
	VALUES (:time_stamp, :node_state, :health_state, :health_metrics,
	:cpus_allocated, :memory_allocated, :gpus_allocated, :jobs_running, :node_id);`

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

			cclog.Debugf("Added node '%s' to database", hostname)
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
	cclog.Debugf("Updated node state for '%s' in database", hostname)
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

// NodeStateWithNode combines a node state row with denormalized node info.
type NodeStateWithNode struct {
	ID              int64  `db:"id"`
	TimeStamp       int64  `db:"time_stamp"`
	NodeState       string `db:"node_state"`
	HealthState     string `db:"health_state"`
	HealthMetrics   string `db:"health_metrics"`
	CpusAllocated   int    `db:"cpus_allocated"`
	MemoryAllocated int64  `db:"memory_allocated"`
	GpusAllocated   int    `db:"gpus_allocated"`
	JobsRunning     int    `db:"jobs_running"`
	Hostname        string `db:"hostname"`
	Cluster         string `db:"cluster"`
	SubCluster      string `db:"subcluster"`
}

// FindNodeStatesBefore returns all node_state rows with time_stamp < cutoff,
// joined with node info for denormalized archiving.
func (r *NodeRepository) FindNodeStatesBefore(cutoff int64) ([]NodeStateWithNode, error) {
	rows, err := sq.Select(
		"node_state.id", "node_state.time_stamp", "node_state.node_state",
		"node_state.health_state", "COALESCE(node_state.health_metrics, '')",
		"node_state.cpus_allocated", "node_state.memory_allocated",
		"node_state.gpus_allocated", "node_state.jobs_running",
		"node.hostname", "node.cluster", "node.subcluster",
	).
		From("node_state").
		Join("node ON node_state.node_id = node.id").
		Where(sq.Lt{"node_state.time_stamp": cutoff}).
		Where("node_state.id NOT IN (SELECT ns2.id FROM node_state ns2 WHERE ns2.time_stamp = (SELECT MAX(ns3.time_stamp) FROM node_state ns3 WHERE ns3.node_id = ns2.node_id))").
		OrderBy("node.cluster ASC", "node.subcluster ASC", "node.hostname ASC", "node_state.time_stamp ASC").
		RunWith(r.DB).Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []NodeStateWithNode
	for rows.Next() {
		var ns NodeStateWithNode
		if err := rows.Scan(&ns.ID, &ns.TimeStamp, &ns.NodeState,
			&ns.HealthState, &ns.HealthMetrics,
			&ns.CpusAllocated, &ns.MemoryAllocated,
			&ns.GpusAllocated, &ns.JobsRunning,
			&ns.Hostname, &ns.Cluster, &ns.SubCluster); err != nil {
			return nil, err
		}
		result = append(result, ns)
	}
	return result, nil
}

// DeleteNodeStatesBefore removes node_state rows with time_stamp < cutoff,
// but always preserves the row with the latest timestamp per node_id.
func (r *NodeRepository) DeleteNodeStatesBefore(cutoff int64) (int64, error) {
	res, err := r.DB.Exec(
		`DELETE FROM node_state WHERE time_stamp < ?
		 AND id NOT IN (
		   SELECT id FROM node_state ns2
		   WHERE ns2.time_stamp = (SELECT MAX(ns3.time_stamp) FROM node_state ns3 WHERE ns3.node_id = ns2.node_id)
		 )`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

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
	page *model.PageRequest,
	order *model.OrderByInput, // Currently unused!
) ([]*schema.Node, error) {
	query, qerr := AccessCheck(ctx,
		sq.Select("node.hostname", "node.cluster", "node.subcluster",
			"node_state.node_state", "node_state.health_state").
			From("node").
			Join("node_state ON node_state.node_id = node.id").
			Where(latestStateCondition()))
	if qerr != nil {
		return nil, qerr
	}

	query = applyNodeFilters(query, filters)
	query = query.OrderBy("node.hostname ASC")

	if page != nil && page.ItemsPerPage != -1 {
		limit := uint64(page.ItemsPerPage)
		query = query.Offset((uint64(page.Page) - 1) * limit).Limit(limit)
	}

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := query.ToSql()
		cclog.Errorf("Error while running query '%s' %v: %v", queryString, queryVars, err)
		return nil, err
	}

	nodes := make([]*schema.Node, 0)
	for rows.Next() {
		node := schema.Node{}
		if err := rows.Scan(&node.Hostname, &node.Cluster, &node.SubCluster,
			&node.NodeState, &node.HealthState); err != nil {
			rows.Close()
			cclog.Warn("Error while scanning rows (QueryNodes)")
			return nil, err
		}
		nodes = append(nodes, &node)
	}

	return nodes, nil
}

// QueryNodesWithMeta returns a list of nodes based on a node filter. It always operates
// on the last state (largest timestamp). It includes both (!) optional JSON column data
func (r *NodeRepository) QueryNodesWithMeta(
	ctx context.Context,
	filters []*model.NodeFilter,
	page *model.PageRequest,
	order *model.OrderByInput, // Currently unused!
) ([]*schema.Node, error) {
	query, qerr := AccessCheck(ctx,
		sq.Select("node.hostname", "node.cluster", "node.subcluster",
			"node_state.node_state", "node_state.health_state",
			"node.meta_data", "node_state.health_metrics").
			From("node").
			Join("node_state ON node_state.node_id = node.id").
			Where(latestStateCondition()))
	if qerr != nil {
		return nil, qerr
	}

	query = applyNodeFilters(query, filters)
	query = query.OrderBy("node.hostname ASC")

	if page != nil && page.ItemsPerPage != -1 {
		limit := uint64(page.ItemsPerPage)
		query = query.Offset((uint64(page.Page) - 1) * limit).Limit(limit)
	}

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := query.ToSql()
		cclog.Errorf("Error while running query '%s' %v: %v", queryString, queryVars, err)
		return nil, err
	}

	nodes := make([]*schema.Node, 0)
	for rows.Next() {
		node := schema.Node{}
		RawMetaData := make([]byte, 0)
		RawMetricHealth := make([]byte, 0)

		if err := rows.Scan(&node.Hostname, &node.Cluster, &node.SubCluster,
			&node.NodeState, &node.HealthState, &RawMetaData, &RawMetricHealth); err != nil {
			rows.Close()
			cclog.Warn("Error while scanning rows (QueryNodes)")
			return nil, err
		}

		if len(RawMetaData) == 0 {
			node.MetaData = nil
		} else {
			metaData := make(map[string]string)
			if err := json.Unmarshal(RawMetaData, &metaData); err != nil {
				cclog.Warn("Error while unmarshaling raw metadata json")
				return nil, err
			}
			node.MetaData = metaData
		}

		if len(RawMetricHealth) == 0 {
			node.HealthData = nil
		} else {
			healthData := make(map[string][]string)
			if err := json.Unmarshal(RawMetricHealth, &healthData); err != nil {
				cclog.Warn("Error while unmarshaling raw healthdata json")
				return nil, err
			}
			node.HealthData = healthData
		}

		nodes = append(nodes, &node)
	}

	return nodes, nil
}

// CountNodes returns the total matched nodes based on a node filter. It always operates
// on the last state (largest timestamp) per node.
func (r *NodeRepository) CountNodes(
	ctx context.Context,
	filters []*model.NodeFilter,
) (int, error) {
	query, qerr := AccessCheck(ctx,
		sq.Select("COUNT(*)").
			From("node").
			Join("node_state ON node_state.node_id = node.id").
			Where(latestStateCondition()))
	if qerr != nil {
		return 0, qerr
	}

	query = applyNodeFilters(query, filters)

	var count int
	if err := query.RunWith(r.stmtCache).QueryRow().Scan(&count); err != nil {
		queryString, queryVars, _ := query.ToSql()
		cclog.Errorf("Error while running query '%s' %v: %v", queryString, queryVars, err)
		return 0, err
	}

	return count, nil
}

func (r *NodeRepository) ListNodes(cluster string) ([]*schema.Node, error) {
	q := sq.Select("node.hostname", "node.cluster", "node.subcluster",
		"node_state.node_state", "node_state.health_state").
		From("node").
		Join("node_state ON node_state.node_id = node.id").
		Where(latestStateCondition()).
		Where("node.cluster = ?", cluster).
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
		if err := rows.Scan(&node.Hostname, &node.Cluster,
			&node.SubCluster, &node.NodeState, &node.HealthState); err != nil {
			cclog.Warn("Error while scanning node list (ListNodes)")
			return nil, err
		}

		nodeList = append(nodeList, node)
	}

	return nodeList, nil
}

func (r *NodeRepository) MapNodes(cluster string) (map[string]string, error) {
	q := sq.Select("node.hostname", "node_state.node_state").
		From("node").
		Join("node_state ON node_state.node_id = node.id").
		Where(latestStateCondition()).
		Where("node.cluster = ?", cluster).
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
		if err := rows.Scan(&hostname, &nodestate); err != nil {
			cclog.Warn("Error while scanning node list (MapNodes)")
			return nil, err
		}

		stateMap[hostname] = nodestate
	}

	return stateMap, nil
}

func (r *NodeRepository) CountStates(ctx context.Context, filters []*model.NodeFilter, column string) ([]*model.NodeStates, error) {
	query, qerr := AccessCheck(ctx,
		sq.Select(column).
			From("node").
			Join("node_state ON node_state.node_id = node.id").
			Where(latestStateCondition()))
	if qerr != nil {
		return nil, qerr
	}

	query = applyNodeFilters(query, filters)

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := query.ToSql()
		cclog.Errorf("Error while running query '%s' %v: %v", queryString, queryVars, err)
		return nil, err
	}

	stateMap := map[string]int{}
	for rows.Next() {
		var state string
		if err := rows.Scan(&state); err != nil {
			rows.Close()
			cclog.Warn("Error while scanning rows (CountStates)")
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
		if f.SubCluster != nil {
			query = buildStringCondition("subcluster", f.SubCluster, query)
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

func (r *NodeRepository) GetNodesForList(
	ctx context.Context,
	cluster string,
	subCluster string,
	stateFilter string,
	nodeFilter string,
	page *model.PageRequest,
) ([]string, map[string]string, int, bool, error) {
	// Init Return Vars
	nodes := make([]string, 0)
	stateMap := make(map[string]string)
	countNodes := 0
	hasNextPage := false

	// Build Filters
	queryFilters := make([]*model.NodeFilter, 0)
	if cluster != "" {
		queryFilters = append(queryFilters, &model.NodeFilter{Cluster: &model.StringInput{Eq: &cluster}})
	}
	if subCluster != "" {
		queryFilters = append(queryFilters, &model.NodeFilter{SubCluster: &model.StringInput{Eq: &subCluster}})
	}
	if nodeFilter != "" && stateFilter != "notindb" {
		queryFilters = append(queryFilters, &model.NodeFilter{Hostname: &model.StringInput{Contains: &nodeFilter}})
	}
	if stateFilter != "all" && stateFilter != "notindb" {
		queryState := schema.SchedulerState(stateFilter)
		queryFilters = append(queryFilters, &model.NodeFilter{SchedulerState: &queryState})
	}
	// if healthFilter != "all" {
	// 	filters = append(filters, &model.NodeFilter{HealthState: &healthFilter})
	// }

	// Special Case: Disable Paging for missing nodes filter, save IPP for later
	var backupItems int
	if stateFilter == "notindb" {
		backupItems = page.ItemsPerPage
		page.ItemsPerPage = -1
	}

	// Query Nodes From DB
	rawNodes, serr := r.QueryNodes(ctx, queryFilters, page, nil) // Order not Used
	if serr != nil {
		cclog.Warn("error while loading node database data (Resolver.NodeMetricsList)")
		return nil, nil, 0, false, serr
	}

	// Intermediate Node Result Info
	for _, node := range rawNodes {
		if node == nil {
			continue
		}
		nodes = append(nodes, node.Hostname)
		stateMap[node.Hostname] = string(node.NodeState)
	}

	// Special Case: Find Nodes not in DB node table but in metricStore only
	if stateFilter == "notindb" {
		// Reapply Original Paging
		page.ItemsPerPage = backupItems
		// Get Nodes From Topology
		var topoNodes []string
		if subCluster != "" {
			scNodes := archive.NodeLists[cluster][subCluster]
			topoNodes = scNodes.PrintList()
		} else {
			subClusterNodeLists := archive.NodeLists[cluster]
			for _, nodeList := range subClusterNodeLists {
				topoNodes = append(topoNodes, nodeList.PrintList()...)
			}
		}
		// Compare to all nodes from cluster/subcluster in DB
		var missingNodes []string
		for _, scanNode := range topoNodes {
			if !slices.Contains(nodes, scanNode) {
				missingNodes = append(missingNodes, scanNode)
			}
		}
		// Filter nodes by name
		if nodeFilter != "" {
			filteredNodesByName := []string{}
			for _, missingNode := range missingNodes {
				if strings.Contains(missingNode, nodeFilter) {
					filteredNodesByName = append(filteredNodesByName, missingNode)
				}
			}
			missingNodes = filteredNodesByName
		}
		// Sort Missing Nodes Alphanumerically
		slices.Sort(missingNodes)
		// Total Missing
		countNodes = len(missingNodes)
		// Apply paging
		if countNodes > page.ItemsPerPage {
			start := (page.Page - 1) * page.ItemsPerPage
			end := start + page.ItemsPerPage
			if end > countNodes {
				end = countNodes
				hasNextPage = false
			} else {
				hasNextPage = true
			}
			nodes = missingNodes[start:end]
		} else {
			nodes = missingNodes
		}

	} else {
		// DB Nodes: Count and derive hasNextPage from count
		var cerr error
		countNodes, cerr = r.CountNodes(ctx, queryFilters)
		if cerr != nil {
			cclog.Warn("error while counting node database data (Resolver.NodeMetricsList)")
			return nil, nil, 0, false, cerr
		}
		hasNextPage = page.Page*page.ItemsPerPage < countNodes
	}

	// Fallback for non-init'd node table in DB; Ignores stateFilter
	if stateFilter == "all" && countNodes == 0 {
		nodes, countNodes, hasNextPage = getNodesFromTopol(cluster, subCluster, nodeFilter, page)
	}

	return nodes, stateMap, countNodes, hasNextPage, nil
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

func getNodesFromTopol(cluster string, subCluster string, nodeFilter string, page *model.PageRequest) ([]string, int, bool) {
	// 0) Init additional vars
	hasNextPage := false
	totalNodes := 0

	// 1) Get list of all nodes
	var topolNodes []string
	if subCluster != "" {
		scNodes := archive.NodeLists[cluster][subCluster]
		topolNodes = scNodes.PrintList()
	} else {
		subClusterNodeLists := archive.NodeLists[cluster]
		for _, nodeList := range subClusterNodeLists {
			topolNodes = append(topolNodes, nodeList.PrintList()...)
		}
	}

	// 2) Filter nodes
	if nodeFilter != "" {
		filteredNodes := []string{}
		for _, node := range topolNodes {
			if strings.Contains(node, nodeFilter) {
				filteredNodes = append(filteredNodes, node)
			}
		}
		topolNodes = filteredNodes
	}

	// 2.1) Count total nodes && Sort nodes -> Sorting invalidated after ccms return ...
	totalNodes = len(topolNodes)
	sort.Strings(topolNodes)

	// 3) Apply paging
	if len(topolNodes) > page.ItemsPerPage {
		start := (page.Page - 1) * page.ItemsPerPage
		end := start + page.ItemsPerPage
		if end >= len(topolNodes) {
			end = len(topolNodes)
			hasNextPage = false
		} else {
			hasNextPage = true
		}
		topolNodes = topolNodes[start:end]
	}

	return topolNodes, totalNodes, hasNextPage
}
