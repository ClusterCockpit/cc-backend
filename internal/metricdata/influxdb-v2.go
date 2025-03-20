// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricdata

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2Api "github.com/influxdata/influxdb-client-go/v2/api"
)

type InfluxDBv2DataRepositoryConfig struct {
	Url     string `json:"url"`
	Token   string `json:"token"`
	Bucket  string `json:"bucket"`
	Org     string `json:"org"`
	SkipTls bool   `json:"skiptls"`
}

type InfluxDBv2DataRepository struct {
	client              influxdb2.Client
	queryClient         influxdb2Api.QueryAPI
	bucket, measurement string
}

func (idb *InfluxDBv2DataRepository) Init(rawConfig json.RawMessage) error {
	var config InfluxDBv2DataRepositoryConfig
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		log.Warn("Error while unmarshaling raw json config")
		return err
	}

	idb.client = influxdb2.NewClientWithOptions(config.Url, config.Token, influxdb2.DefaultOptions().SetTLSConfig(&tls.Config{InsecureSkipVerify: config.SkipTls}))
	idb.queryClient = idb.client.QueryAPI(config.Org)
	idb.bucket = config.Bucket

	return nil
}

func (idb *InfluxDBv2DataRepository) formatTime(t time.Time) string {
	return t.Format(time.RFC3339) // Like “2006-01-02T15:04:05Z07:00”
}

func (idb *InfluxDBv2DataRepository) epochToTime(epoch int64) time.Time {
	return time.Unix(epoch, 0)
}

func (idb *InfluxDBv2DataRepository) LoadData(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context,
	resolution int) (schema.JobData, error) {

	log.Infof("InfluxDB 2 Backend: Resolution Scaling not Implemented, will return default timestep. Requested Resolution %d", resolution)

	measurementsConds := make([]string, 0, len(metrics))
	for _, m := range metrics {
		measurementsConds = append(measurementsConds, fmt.Sprintf(`r["_measurement"] == "%s"`, m))
	}
	measurementsCond := strings.Join(measurementsConds, " or ")

	hostsConds := make([]string, 0, len(job.Resources))
	for _, h := range job.Resources {
		if h.HWThreads != nil || h.Accelerators != nil {
			// TODO
			return nil, errors.New("METRICDATA/INFLUXV2 > the InfluxDB metric data repository does not yet support HWThreads or Accelerators")
		}
		hostsConds = append(hostsConds, fmt.Sprintf(`r["hostname"] == "%s"`, h.Hostname))
	}
	hostsCond := strings.Join(hostsConds, " or ")

	jobData := make(schema.JobData) // Empty Schema: map[<string>FIELD]map[<MetricScope>SCOPE]<*JobMetric>METRIC
	// Requested Scopes
	for _, scope := range scopes {
		query := ""
		switch scope {
		case "node":
			// Get Finest Granularity, Groupy By Measurement and Hostname (== Metric / Node), Calculate Mean for 60s windows <-- Resolution could be added here?
			// log.Info("Scope 'node' requested. ")
			query = fmt.Sprintf(`
								from(bucket: "%s")
								|> range(start: %s, stop: %s)
								|> filter(fn: (r) => (%s) and (%s) )
								|> drop(columns: ["_start", "_stop"])
								|> group(columns: ["hostname", "_measurement"])
		            |> aggregateWindow(every: 60s, fn: mean)
								|> drop(columns: ["_time"])`,
				idb.bucket,
				idb.formatTime(job.StartTime), idb.formatTime(idb.epochToTime(job.StartTimeUnix+int64(job.Duration)+int64(1))),
				measurementsCond, hostsCond)
		case "socket":
			log.Info("Scope 'socket' requested, but not yet supported: Will return 'node' scope only. ")
			continue
		case "core":
			log.Info(" Scope 'core' requested, but not yet supported: Will return 'node' scope only. ")
			continue
			// Get Finest Granularity only, Set NULL to 0.0
			// query = fmt.Sprintf(`
			//  	from(bucket: "%s")
			//  	|> range(start: %s, stop: %s)
			//  	|> filter(fn: (r) => %s )
			//  	|> filter(fn: (r) => %s )
			//  	|> drop(columns: ["_start", "_stop", "cluster"])
			//  	|> map(fn: (r) => (if exists r._value then {r with _value: r._value} else {r with _value: 0.0}))`,
			//  	idb.bucket,
			//  	idb.formatTime(job.StartTime), idb.formatTime(idb.epochToTime(job.StartTimeUnix + int64(job.Duration) + int64(1) )),
			//  	measurementsCond, hostsCond)
		case "hwthread":
			log.Info(" Scope 'hwthread' requested, but not yet supported: Will return 'node' scope only. ")
			continue
		case "accelerator":
			log.Info(" Scope 'accelerator' requested, but not yet supported: Will return 'node' scope only. ")
			continue
		default:
			log.Infof("Unknown scope '%s' requested: Will return 'node' scope.", scope)
			continue
			// return nil, errors.New("METRICDATA/INFLUXV2 > the InfluxDB metric data repository does not yet support other scopes than 'node'")
		}

		rows, err := idb.queryClient.Query(ctx, query)
		if err != nil {
			log.Error("Error while performing query")
			return nil, err
		}

		// Init Metrics: Only Node level now -> TODO: Matching /check on scope level ...
		for _, metric := range metrics {
			jobMetric, ok := jobData[metric]
			if !ok {
				mc := archive.GetMetricConfig(job.Cluster, metric)
				jobMetric = map[schema.MetricScope]*schema.JobMetric{
					scope: { // uses scope var from above!
						Unit:             mc.Unit,
						Timestep:         mc.Timestep,
						Series:           make([]schema.Series, 0, len(job.Resources)),
						StatisticsSeries: nil, // Should be: &schema.StatsSeries{},
					},
				}
			}
			jobData[metric] = jobMetric
		}

		// Process Result: Time-Data
		field, host, hostSeries := "", "", schema.Series{}
		// typeId := 0
		switch scope {
		case "node":
			for rows.Next() {
				row := rows.Record()
				if host == "" || host != row.ValueByKey("hostname").(string) || rows.TableChanged() {
					if host != "" {
						// Append Series before reset
						jobData[field][scope].Series = append(jobData[field][scope].Series, hostSeries)
					}
					field, host = row.Measurement(), row.ValueByKey("hostname").(string)
					hostSeries = schema.Series{
						Hostname:   host,
						Statistics: schema.MetricStatistics{}, //TODO Add Statistics
						Data:       make([]schema.Float, 0),
					}
				}
				val, ok := row.Value().(float64)
				if ok {
					hostSeries.Data = append(hostSeries.Data, schema.Float(val))
				} else {
					hostSeries.Data = append(hostSeries.Data, schema.Float(0))
				}
			}
		case "socket":
			continue
		case "accelerator":
			continue
		case "hwthread":
			// See below @ core
			continue
		case "core":
			continue
			// Include Series.Id in hostSeries
			// for rows.Next() {
			// 		row := rows.Record()
			// 		if ( host == "" || host != row.ValueByKey("hostname").(string) || typeId != row.ValueByKey("type-id").(int) || rows.TableChanged() ) {
			// 		 		if ( host != "" ) {
			// 						// Append Series before reset
			// 		 		  	jobData[field][scope].Series = append(jobData[field][scope].Series, hostSeries)
			// 		 		}
			// 		 		field, host, typeId = row.Measurement(), row.ValueByKey("hostname").(string), row.ValueByKey("type-id").(int)
			// 		 		hostSeries  = schema.Series{
			// 		 				Hostname:   host,
			// 						Id:					&typeId,
			// 		 				Statistics: nil,
			// 		 				Data:       make([]schema.Float, 0),
			// 		 		}
			// 		}
			// 		val := row.Value().(float64)
			// 		hostSeries.Data = append(hostSeries.Data, schema.Float(val))
			// }
		default:
			log.Infof("Unknown scope '%s' requested: Will return 'node' scope.", scope)
			continue
			// return nil, errors.New("the InfluxDB metric data repository does not yet support other scopes than 'node, core'")
		}
		// Append last Series
		jobData[field][scope].Series = append(jobData[field][scope].Series, hostSeries)
	}

	// Get Stats
	stats, err := idb.LoadStats(job, metrics, ctx)
	if err != nil {
		log.Warn("Error while loading statistics")
		return nil, err
	}

	for _, scope := range scopes {
		if scope == "node" { // No 'socket/core' support yet
			for metric, nodes := range stats {
				for node, stats := range nodes {
					for index, _ := range jobData[metric][scope].Series {
						if jobData[metric][scope].Series[index].Hostname == node {
							jobData[metric][scope].Series[index].Statistics = schema.MetricStatistics{Avg: stats.Avg, Min: stats.Min, Max: stats.Max}
						}
					}
				}
			}
		}
	}

	return jobData, nil
}

func (idb *InfluxDBv2DataRepository) LoadStats(
	job *schema.Job,
	metrics []string,
	ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {

	stats := map[string]map[string]schema.MetricStatistics{}

	hostsConds := make([]string, 0, len(job.Resources))
	for _, h := range job.Resources {
		if h.HWThreads != nil || h.Accelerators != nil {
			// TODO
			return nil, errors.New("METRICDATA/INFLUXV2 > the InfluxDB metric data repository does not yet support HWThreads or Accelerators")
		}
		hostsConds = append(hostsConds, fmt.Sprintf(`r["hostname"] == "%s"`, h.Hostname))
	}
	hostsCond := strings.Join(hostsConds, " or ")

	// lenMet := len(metrics)

	for _, metric := range metrics {
		// log.Debugf("<< You are here: %s (Index %d of %d metrics)", metric, index, lenMet)

		query := fmt.Sprintf(`
				  data = from(bucket: "%s")
				  |> range(start: %s, stop: %s)
				  |> filter(fn: (r) => r._measurement == "%s" and r._field == "value" and (%s))
				  union(tables: [data |> mean(column: "_value") |> set(key: "_field", value: "avg"),
				                 data |>  min(column: "_value") |> set(key: "_field", value: "min"),
				                 data |>  max(column: "_value") |> set(key: "_field", value: "max")])
				  |> pivot(rowKey: ["hostname"], columnKey: ["_field"], valueColumn: "_value")
				  |> group()`,
			idb.bucket,
			idb.formatTime(job.StartTime), idb.formatTime(idb.epochToTime(job.StartTimeUnix+int64(job.Duration)+int64(1))),
			metric, hostsCond)

		rows, err := idb.queryClient.Query(ctx, query)
		if err != nil {
			log.Error("Error while performing query")
			return nil, err
		}

		nodes := map[string]schema.MetricStatistics{}
		for rows.Next() {
			row := rows.Record()
			host := row.ValueByKey("hostname").(string)

			avg, avgok := row.ValueByKey("avg").(float64)
			if !avgok {
				// log.Debugf(">> Assertion error for metric %s, statistic AVG. Expected 'float64', got %v", metric, avg)
				avg = 0.0
			}
			min, minok := row.ValueByKey("min").(float64)
			if !minok {
				// log.Debugf(">> Assertion error for metric %s, statistic MIN. Expected 'float64', got %v", metric, min)
				min = 0.0
			}
			max, maxok := row.ValueByKey("max").(float64)
			if !maxok {
				// log.Debugf(">> Assertion error for metric %s, statistic MAX. Expected 'float64', got %v", metric, max)
				max = 0.0
			}

			nodes[host] = schema.MetricStatistics{
				Avg: avg,
				Min: min,
				Max: max,
			}
		}
		stats[metric] = nodes
	}

	return stats, nil
}

// Used in Job-View StatsTable
// UNTESTED
func (idb *InfluxDBv2DataRepository) LoadScopedStats(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context) (schema.ScopedJobStats, error) {

	// Assumption: idb.loadData() only returns series node-scope - use node scope for statsTable
	scopedJobStats := make(schema.ScopedJobStats)
	data, err := idb.LoadData(job, metrics, []schema.MetricScope{schema.MetricScopeNode}, ctx, 0 /*resolution here*/)
	if err != nil {
		log.Warn("Error while loading job for scopedJobStats")
		return nil, err
	}

	for metric, metricData := range data {
		for _, scope := range scopes {
			if scope != schema.MetricScopeNode {
				logOnce.Do(func() {
					log.Infof("Note: Scope '%s' requested, but not yet supported: Will return 'node' scope only.", scope)
				})
				continue
			}

			if _, ok := scopedJobStats[metric]; !ok {
				scopedJobStats[metric] = make(map[schema.MetricScope][]*schema.ScopedStats)
			}

			if _, ok := scopedJobStats[metric][scope]; !ok {
				scopedJobStats[metric][scope] = make([]*schema.ScopedStats, 0)
			}

			for _, series := range metricData[scope].Series {
				scopedJobStats[metric][scope] = append(scopedJobStats[metric][scope], &schema.ScopedStats{
					Hostname: series.Hostname,
					Data:     &series.Statistics,
				})
			}
		}
	}

	return scopedJobStats, nil
}

// Used in Systems-View @ Node-Overview
// UNTESTED
func (idb *InfluxDBv2DataRepository) LoadNodeData(
	cluster string,
	metrics, nodes []string,
	scopes []schema.MetricScope,
	from, to time.Time,
	ctx context.Context) (map[string]map[string][]*schema.JobMetric, error) {

	// Note: scopes[] Array will be ignored, only return node scope

	// CONVERT ARGS TO INFLUX
	measurementsConds := make([]string, 0)
	for _, m := range metrics {
		measurementsConds = append(measurementsConds, fmt.Sprintf(`r["_measurement"] == "%s"`, m))
	}
	measurementsCond := strings.Join(measurementsConds, " or ")

	hostsConds := make([]string, 0)
	if nodes == nil {
		var allNodes []string
		subClusterNodeLists := archive.NodeLists[cluster]
		for _, nodeList := range subClusterNodeLists {
			allNodes = append(nodes, nodeList.PrintList()...)
		}
		for _, node := range allNodes {
			nodes = append(nodes, node)
			hostsConds = append(hostsConds, fmt.Sprintf(`r["hostname"] == "%s"`, node))
		}
	} else {
		for _, node := range nodes {
			hostsConds = append(hostsConds, fmt.Sprintf(`r["hostname"] == "%s"`, node))
		}
	}
	hostsCond := strings.Join(hostsConds, " or ")

	// BUILD AND PERFORM QUERY
	query := fmt.Sprintf(`
						from(bucket: "%s")
						|> range(start: %s, stop: %s)
						|> filter(fn: (r) => (%s) and (%s) )
						|> drop(columns: ["_start", "_stop"])
						|> group(columns: ["hostname", "_measurement"])
			|> aggregateWindow(every: 60s, fn: mean)
						|> drop(columns: ["_time"])`,
		idb.bucket,
		idb.formatTime(from), idb.formatTime(to),
		measurementsCond, hostsCond)

	rows, err := idb.queryClient.Query(ctx, query)
	if err != nil {
		log.Error("Error while performing query")
		return nil, err
	}

	// HANDLE QUERY RETURN
	// Collect Float Arrays for Node@Metric -> No Scope Handling!
	influxData := make(map[string]map[string][]schema.Float)
	for rows.Next() {
		row := rows.Record()
		host, field := row.ValueByKey("hostname").(string), row.Measurement()

		influxHostData, ok := influxData[host]
		if !ok {
			influxHostData = make(map[string][]schema.Float)
			influxData[host] = influxHostData
		}

		influxFieldData, ok := influxData[host][field]
		if !ok {
			influxFieldData = make([]schema.Float, 0)
			influxData[host][field] = influxFieldData
		}

		val, ok := row.Value().(float64)
		if ok {
			influxData[host][field] = append(influxData[host][field], schema.Float(val))
		} else {
			influxData[host][field] = append(influxData[host][field], schema.Float(0))
		}
	}

	// BUILD FUNCTION RETURN
	data := make(map[string]map[string][]*schema.JobMetric)
	for node, metricData := range influxData {

		nodeData, ok := data[node]
		if !ok {
			nodeData = make(map[string][]*schema.JobMetric)
			data[node] = nodeData
		}

		for metric, floatArray := range metricData {
			avg, min, max := 0.0, 0.0, 0.0
			for _, val := range floatArray {
				avg += float64(val)
				min = math.Min(min, float64(val))
				max = math.Max(max, float64(val))
			}

			stats := schema.MetricStatistics{
				Avg: (math.Round((avg/float64(len(floatArray)))*100) / 100),
				Min: (math.Round(min*100) / 100),
				Max: (math.Round(max*100) / 100),
			}

			mc := archive.GetMetricConfig(cluster, metric)
			nodeData[metric] = append(nodeData[metric], &schema.JobMetric{
				Unit:     mc.Unit,
				Timestep: mc.Timestep,
				Series: []schema.Series{
					{
						Hostname:   node,
						Statistics: stats,
						Data:       floatArray,
					},
				},
			})
		}
	}

	return data, nil
}

// Used in Systems-View @ Node-List
// UNTESTED
func (idb *InfluxDBv2DataRepository) LoadNodeListData(
	cluster, subCluster, nodeFilter string,
	metrics []string,
	scopes []schema.MetricScope,
	resolution int,
	from, to time.Time,
	page *model.PageRequest,
	ctx context.Context,
) (map[string]schema.JobData, int, bool, error) {

	// Assumption: idb.loadData() only returns series node-scope - use node scope for NodeList

	// 0) Init additional vars
	var totalNodes int = 0
	var hasNextPage bool = false

	// 1) Get list of all nodes
	var nodes []string
	if subCluster != "" {
		scNodes := archive.NodeLists[cluster][subCluster]
		nodes = scNodes.PrintList()
	} else {
		subClusterNodeLists := archive.NodeLists[cluster]
		for _, nodeList := range subClusterNodeLists {
			nodes = append(nodes, nodeList.PrintList()...)
		}
	}

	// 2) Filter nodes
	if nodeFilter != "" {
		filteredNodes := []string{}
		for _, node := range nodes {
			if strings.Contains(node, nodeFilter) {
				filteredNodes = append(filteredNodes, node)
			}
		}
		nodes = filteredNodes
	}

	// 2.1) Count total nodes && Sort nodes -> Sorting invalidated after return ...
	totalNodes = len(nodes)
	sort.Strings(nodes)

	// 3) Apply paging
	if len(nodes) > page.ItemsPerPage {
		start := (page.Page - 1) * page.ItemsPerPage
		end := start + page.ItemsPerPage
		if end > len(nodes) {
			end = len(nodes)
			hasNextPage = false
		} else {
			hasNextPage = true
		}
		nodes = nodes[start:end]
	}

	// 4) Fetch And Convert Data, use idb.LoadNodeData() for query

	rawNodeData, err := idb.LoadNodeData(cluster, metrics, nodes, scopes, from, to, ctx)
	if err != nil {
		log.Error(fmt.Sprintf("Error while loading influx nodeData for nodeListData %#v\n", err))
		return nil, totalNodes, hasNextPage, err
	}

	data := make(map[string]schema.JobData)
	for node, nodeData := range rawNodeData {
		// Init Nested Map Data Structures If Not Found
		hostData, ok := data[node]
		if !ok {
			hostData = make(schema.JobData)
			data[node] = hostData
		}

		for metric, nodeMetricData := range nodeData {
			metricData, ok := hostData[metric]
			if !ok {
				metricData = make(map[schema.MetricScope]*schema.JobMetric)
				data[node][metric] = metricData
			}

			data[node][metric][schema.MetricScopeNode] = nodeMetricData[0] // Only Node Scope Returned from loadNodeData
		}
	}

	return data, totalNodes, hasNextPage, nil
}
