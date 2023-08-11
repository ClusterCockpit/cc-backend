// Copyright (C) 2022 DKRZ
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package metricdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promcfg "github.com/prometheus/common/config"
	promm "github.com/prometheus/common/model"
)

type PrometheusDataRepositoryConfig struct {
	Url       string            `json:"url"`
	Username  string            `json:"username,omitempty"`
	Suffix    string            `json:"suffix,omitempty"`
	Templates map[string]string `json:"query-templates"`
}

type PrometheusDataRepository struct {
	client      promapi.Client
	queryClient promv1.API
	suffix      string
	templates   map[string]*template.Template
}

type PromQLArgs struct {
	Nodes string
}

type Trie map[rune]Trie

var logOnce sync.Once

func contains(s []schema.MetricScope, str schema.MetricScope) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func MinMaxMean(data []schema.Float) (float64, float64, float64) {
	if len(data) == 0 {
		return 0.0, 0.0, 0.0
	}
	min := math.MaxFloat64
	max := -math.MaxFloat64
	var sum float64
	var n float64
	for _, val := range data {
		if val.IsNaN() {
			continue
		}
		sum += float64(val)
		n += 1
		if float64(val) > max {
			max = float64(val)
		}
		if float64(val) < min {
			min = float64(val)
		}
	}
	return min, max, sum / n
}

// Rewritten from
// https://github.com/ermanh/trieregex/blob/master/trieregex/trieregex.py
func nodeRegex(nodes []string) string {
	root := Trie{}
	// add runes of each compute node to trie
	for _, node := range nodes {
		_trie := root
		for _, c := range node {
			if _, ok := _trie[c]; !ok {
				_trie[c] = Trie{}
			}
			_trie = _trie[c]
		}
		_trie['*'] = Trie{}
	}
	// recursively build regex from rune trie
	var trieRegex func(trie Trie, reset bool) string
	trieRegex = func(trie Trie, reset bool) string {
		if reset == true {
			trie = root
		}
		if len(trie) == 0 {
			return ""
		}
		if len(trie) == 1 {
			for key, _trie := range trie {
				if key == '*' {
					return ""
				}
				return regexp.QuoteMeta(string(key)) + trieRegex(_trie, false)
			}
		} else {
			sequences := []string{}
			for key, _trie := range trie {
				if key != '*' {
					sequences = append(sequences, regexp.QuoteMeta(string(key))+trieRegex(_trie, false))
				}
			}
			sort.Slice(sequences, func(i, j int) bool {
				return (-len(sequences[i]) < -len(sequences[j])) || (sequences[i] < sequences[j])
			})
			var result string
			// single edge from this tree node
			if len(sequences) == 1 {
				result = sequences[0]
				if len(result) > 1 {
					result = "(?:" + result + ")"
				}
				// multiple edges, each length 1
			} else if s := strings.Join(sequences, ""); len(s) == len(sequences) {
				// char or numeric range
				if len(s)-1 == int(s[len(s)-1])-int(s[0]) {
					result = fmt.Sprintf("[%c-%c]", s[0], s[len(s)-1])
					// char or numeric set
				} else {
					result = "[" + s + "]"
				}
				// multiple edges of different lengths
			} else {
				result = "(?:" + strings.Join(sequences, "|") + ")"
			}
			if _, ok := trie['*']; ok {
				result += "?"
			}
			return result
		}
		return ""
	}
	return trieRegex(root, true)
}

func (pdb *PrometheusDataRepository) Init(rawConfig json.RawMessage) error {
	var config PrometheusDataRepositoryConfig
	// parse config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		log.Warn("Error while unmarshaling raw json config")
		return err
	}
	// support basic authentication
	var rt http.RoundTripper = nil
	if prom_pw := os.Getenv("PROMETHEUS_PASSWORD"); prom_pw != "" && config.Username != "" {
		prom_pw := promcfg.Secret(prom_pw)
		rt = promcfg.NewBasicAuthRoundTripper(config.Username, prom_pw, "", promapi.DefaultRoundTripper)
	} else {
		if config.Username != "" {
			return errors.New("METRICDATA/PROMETHEUS > Prometheus username provided, but PROMETHEUS_PASSWORD not set.")
		}
	}
	// init client
	client, err := promapi.NewClient(promapi.Config{
		Address:      config.Url,
		RoundTripper: rt,
	})
	if err != nil {
		log.Error("Error while initializing new prometheus client")
		return err
	}
	// init query client
	pdb.client = client
	pdb.queryClient = promv1.NewAPI(pdb.client)
	// site config
	pdb.suffix = config.Suffix
	// init query templates
	pdb.templates = make(map[string]*template.Template)
	for metric, templ := range config.Templates {
		pdb.templates[metric], err = template.New(metric).Parse(templ)
		if err == nil {
			log.Debugf("Added PromQL template for %s: %s", metric, templ)
		} else {
			log.Warnf("Failed to parse PromQL template %s for metric %s", templ, metric)
		}
	}
	return nil
}

// TODO: respect scope argument
func (pdb *PrometheusDataRepository) FormatQuery(
	metric string,
	scope schema.MetricScope,
	nodes []string,
	cluster string) (string, error) {

	args := PromQLArgs{}
	if len(nodes) > 0 {
		args.Nodes = fmt.Sprintf("(%s)%s", nodeRegex(nodes), pdb.suffix)
	} else {
		args.Nodes = fmt.Sprintf(".*%s", pdb.suffix)
	}

	buf := &bytes.Buffer{}
	if templ, ok := pdb.templates[metric]; ok {
		err := templ.Execute(buf, args)
		if err != nil {
			return "", errors.New(fmt.Sprintf("METRICDATA/PROMETHEUS > Error compiling template %v", templ))
		} else {
			query := buf.String()
			log.Debugf("PromQL: %s", query)
			return query, nil
		}
	} else {
		return "", errors.New(fmt.Sprintf("METRICDATA/PROMETHEUS > No PromQL for metric %s configured.", metric))
	}
}

// Convert PromAPI row to CC schema.Series
func (pdb *PrometheusDataRepository) RowToSeries(
	from time.Time,
	step int64,
	steps int64,
	row *promm.SampleStream) schema.Series {
	ts := from.Unix()
	hostname := strings.TrimSuffix(string(row.Metric["exported_instance"]), pdb.suffix)
	// init array of expected length with NaN
	values := make([]schema.Float, steps+1)
	for i, _ := range values {
		values[i] = schema.NaN
	}
	// copy recorded values from prom sample pair
	for _, v := range row.Values {
		idx := (v.Timestamp.Unix() - ts) / step
		values[idx] = schema.Float(v.Value)
	}
	min, max, mean := MinMaxMean(values)
	// output struct
	return schema.Series{
		Hostname: hostname,
		Data:     values,
		Statistics: schema.MetricStatistics{
			Avg: mean,
			Min: min,
			Max: max,
		},
	}
}

func (pdb *PrometheusDataRepository) LoadData(
	job *schema.Job,
	metrics []string,
	scopes []schema.MetricScope,
	ctx context.Context) (schema.JobData, error) {

	// TODO respect requested scope
	if len(scopes) == 0 || !contains(scopes, schema.MetricScopeNode) {
		scopes = append(scopes, schema.MetricScopeNode)
	}

	jobData := make(schema.JobData)
	// parse job specs
	nodes := make([]string, len(job.Resources))
	for i, resource := range job.Resources {
		nodes[i] = resource.Hostname
	}
	from := job.StartTime
	to := job.StartTime.Add(time.Duration(job.Duration) * time.Second)

	for _, scope := range scopes {
		if scope != schema.MetricScopeNode {
			logOnce.Do(func() {
				log.Infof("Scope '%s' requested, but not yet supported: Will return 'node' scope only.", scope)
			})
			continue
		}

		for _, metric := range metrics {
			metricConfig := archive.GetMetricConfig(job.Cluster, metric)
			if metricConfig == nil {
				log.Warnf("Error in LoadData: Metric %s for cluster %s not configured", metric, job.Cluster)
				return nil, errors.New("Prometheus config error")
			}
			query, err := pdb.FormatQuery(metric, scope, nodes, job.Cluster)
			if err != nil {
				log.Warn("Error while formatting prometheus query")
				return nil, err
			}

			// ranged query over all job nodes
			r := promv1.Range{
				Start: from,
				End:   to,
				Step:  time.Duration(metricConfig.Timestep * 1e9),
			}
			result, warnings, err := pdb.queryClient.QueryRange(ctx, query, r)

			if err != nil {
				log.Errorf("Prometheus query error in LoadData: %v\nQuery: %s", err, query)
				return nil, errors.New("Prometheus query error")
			}
			if len(warnings) > 0 {
				log.Warnf("Warnings: %v\n", warnings)
			}

			// init data structures
			if _, ok := jobData[metric]; !ok {
				jobData[metric] = make(map[schema.MetricScope]*schema.JobMetric)
			}
			jobMetric, ok := jobData[metric][scope]
			if !ok {
				jobMetric = &schema.JobMetric{
					Unit:     metricConfig.Unit,
					Timestep: metricConfig.Timestep,
					Series:   make([]schema.Series, 0),
				}
			}
			step := int64(metricConfig.Timestep)
			steps := int64(to.Sub(from).Seconds()) / step
			// iter rows of host, metric, values
			for _, row := range result.(promm.Matrix) {
				jobMetric.Series = append(jobMetric.Series,
					pdb.RowToSeries(from, step, steps, row))
			}
			// only add metric if at least one host returned data
			if !ok && len(jobMetric.Series) > 0{
				jobData[metric][scope] = jobMetric
			}
			// sort by hostname to get uniform coloring
			sort.Slice(jobMetric.Series, func(i, j int) bool {
				return (jobMetric.Series[i].Hostname < jobMetric.Series[j].Hostname)
			})
		}
	}
	return jobData, nil
}

// TODO change implementation to precomputed/cached stats
func (pdb *PrometheusDataRepository) LoadStats(
	job *schema.Job,
	metrics []string,
	ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {

	// map of metrics of nodes of stats
	stats := map[string]map[string]schema.MetricStatistics{}

	data, err := pdb.LoadData(job, metrics, []schema.MetricScope{schema.MetricScopeNode}, ctx)
	if err != nil {
		log.Warn("Error while loading job for stats")
		return nil, err
	}
	for metric, metricData := range data {
		stats[metric] = make(map[string]schema.MetricStatistics)
		for _, series := range metricData[schema.MetricScopeNode].Series {
			stats[metric][series.Hostname] = series.Statistics
		}
	}

	return stats, nil
}

func (pdb *PrometheusDataRepository) LoadNodeData(
	cluster string,
	metrics, nodes []string,
	scopes []schema.MetricScope,
	from, to time.Time,
	ctx context.Context) (map[string]map[string][]*schema.JobMetric, error) {
	t0 := time.Now()
	// Map of hosts of metrics of value slices
	data := make(map[string]map[string][]*schema.JobMetric)
	// query db for each metric
	// TODO: scopes seems to be always empty
	if len(scopes) == 0 || !contains(scopes, schema.MetricScopeNode) {
		scopes = append(scopes, schema.MetricScopeNode)
	}
	for _, scope := range scopes {
		if scope != schema.MetricScopeNode {
			logOnce.Do(func() {
				log.Infof("Note: Scope '%s' requested, but not yet supported: Will return 'node' scope only.", scope)
			})
			continue
		}
		for _, metric := range metrics {
			metricConfig := archive.GetMetricConfig(cluster, metric)
			if metricConfig == nil {
				log.Warnf("Error in LoadNodeData: Metric %s for cluster %s not configured", metric, cluster)
				return nil, errors.New("Prometheus config error")
			}
			query, err := pdb.FormatQuery(metric, scope, nodes, cluster)
			if err != nil {
				log.Warn("Error while formatting prometheus query")
				return nil, err
			}

			// ranged query over all nodes
			r := promv1.Range{
				Start: from,
				End:   to,
				Step:  time.Duration(metricConfig.Timestep * 1e9),
			}
			result, warnings, err := pdb.queryClient.QueryRange(ctx, query, r)

			if err != nil {
				log.Errorf("Prometheus query error in LoadNodeData: %v\n", err)
				return nil, errors.New("Prometheus query error")
			}
			if len(warnings) > 0 {
				log.Warnf("Warnings: %v\n", warnings)
			}

			step := int64(metricConfig.Timestep)
			steps := int64(to.Sub(from).Seconds()) / step

			// iter rows of host, metric, values
			for _, row := range result.(promm.Matrix) {
				hostname := strings.TrimSuffix(string(row.Metric["exported_instance"]), pdb.suffix)
				hostdata, ok := data[hostname]
				if !ok {
					hostdata = make(map[string][]*schema.JobMetric)
					data[hostname] = hostdata
				}
				// output per host and metric
				hostdata[metric] = append(hostdata[metric], &schema.JobMetric{
					Unit:     metricConfig.Unit,
					Timestep: metricConfig.Timestep,
					Series:   []schema.Series{pdb.RowToSeries(from, step, steps, row)},
				},
				)
			}
		}
	}
	t1 := time.Since(t0)
	log.Debugf("LoadNodeData of %v nodes took %s", len(data), t1)
	return data, nil
}
