// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package tagger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

const (
	// defaultJobClassConfigPath is the default path for job classification configuration
	defaultJobClassConfigPath = "./var/tagger/jobclasses"
	// tagTypeJobClass is the tag type identifier for job classification tags
	tagTypeJobClass = "jobClass"
	// jobClassConfigDirMatch is the directory name used for matching filesystem events
	jobClassConfigDirMatch = "jobclasses"
	// parametersFileName is the name of the parameters configuration file
	parametersFileName = "parameters.json"
)

// Variable defines a named expression that can be computed and reused in rules.
// Variables are evaluated before the main rule and their results are added to the environment.
type Variable struct {
	// Name is the variable identifier used in rule expressions
	Name string `json:"name"`
	// Expr is the expression to evaluate (must return a numeric value)
	Expr string `json:"expr"`
}

type ruleVariable struct {
	name string
	expr *vm.Program
}

// RuleFormat defines the JSON structure for job classification rules.
// Each rule specifies requirements, metrics to analyze, variables to compute,
// and the final rule expression that determines if the job matches the classification.
type RuleFormat struct {
	// Name is a human-readable description of the rule
	Name string `json:"name"`
	// Tag is the classification tag to apply if the rule matches
	Tag string `json:"tag"`
	// Parameters are shared values referenced in the rule (e.g., thresholds)
	Parameters []string `json:"parameters"`
	// Metrics are the job metrics required for this rule (e.g., "cpu_load", "mem_used")
	Metrics []string `json:"metrics"`
	// Requirements are boolean expressions that must be true for the rule to apply
	Requirements []string `json:"requirements"`
	// Variables are computed values used in the rule expression
	Variables []Variable `json:"variables"`
	// Rule is the boolean expression that determines if the job matches
	Rule string `json:"rule"`
	// Hint is a template string that generates a message when the rule matches
	Hint string `json:"hint"`
}

type ruleInfo struct {
	env          map[string]any
	metrics      []string
	requirements []*vm.Program
	variables    []ruleVariable
	rule         *vm.Program
	hint         *template.Template
}

// JobRepository defines the interface for job database operations needed by the tagger.
// This interface allows for easier testing and decoupling from the concrete repository implementation.
type JobRepository interface {
	// HasTag checks if a job already has a specific tag
	HasTag(jobID int64, tagType string, tagName string) bool
	// AddTagOrCreateDirect adds a tag to a job or creates it if it doesn't exist
	AddTagOrCreateDirect(jobID int64, tagType string, tagName string) (tagID int64, err error)
	// UpdateMetadata updates job metadata with a key-value pair
	UpdateMetadata(job *schema.Job, key, val string) (err error)
}

// JobClassTagger classifies jobs based on configurable rules that evaluate job metrics and properties.
// Rules are loaded from an external configuration directory and can be dynamically reloaded when files change.
// When a job matches a rule, it is tagged with the corresponding classification and an optional hint message.
type JobClassTagger struct {
	// rules maps classification tags to their compiled rule information
	rules map[string]ruleInfo
	// parameters are shared values (e.g., thresholds) used across multiple rules
	parameters map[string]any
	// tagType is the type of tag ("jobClass")
	tagType string
	// cfgPath is the path to watch for configuration changes
	cfgPath string
	// repo provides access to job database operations
	repo JobRepository
	// getStatistics retrieves job statistics for analysis
	getStatistics func(job *schema.Job) (map[string]schema.JobStatistics, error)
	// getMetricConfig retrieves metric configuration (limits) for a cluster
	getMetricConfig func(cluster, subCluster string) map[string]*schema.Metric
}

func (t *JobClassTagger) prepareRule(b []byte, fns string) {
	var rule RuleFormat
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&rule); err != nil {
		cclog.Warn("Error while decoding raw job meta json")
		return
	}

	ri := ruleInfo{}
	ri.env = make(map[string]any)
	ri.metrics = make([]string, 0)
	ri.requirements = make([]*vm.Program, 0)
	ri.variables = make([]ruleVariable, 0)

	// check if all required parameters are available
	for _, p := range rule.Parameters {
		param, ok := t.parameters[p]
		if !ok {
			cclog.Warnf("prepareRule() > missing parameter %s in rule %s", p, fns)
			return
		}
		ri.env[p] = param
	}

	// set all required metrics
	ri.metrics = append(ri.metrics, rule.Metrics...)

	// compile requirements
	for _, r := range rule.Requirements {
		req, err := expr.Compile(r, expr.AsBool())
		if err != nil {
			cclog.Errorf("error compiling requirement %s: %#v", r, err)
			return
		}
		ri.requirements = append(ri.requirements, req)
	}

	// compile variables
	for _, v := range rule.Variables {
		req, err := expr.Compile(v.Expr, expr.AsFloat64())
		if err != nil {
			cclog.Errorf("error compiling requirement %s: %#v", v.Name, err)
			return
		}
		ri.variables = append(ri.variables, ruleVariable{name: v.Name, expr: req})
	}

	// compile rule
	exp, err := expr.Compile(rule.Rule, expr.AsBool())
	if err != nil {
		cclog.Errorf("error compiling rule %s: %#v", fns, err)
		return
	}
	ri.rule = exp

	// prepare hint template
	ri.hint, err = template.New(fns).Parse(rule.Hint)
	if err != nil {
		cclog.Errorf("error processing template %s: %#v", fns, err)
	}
	cclog.Infof("prepareRule() > processing %s with %d requirements and %d variables", fns, len(ri.requirements), len(ri.variables))

	t.rules[rule.Tag] = ri
}

// EventMatch checks if a filesystem event should trigger configuration reload.
// It returns true if the event path contains "jobclasses".
func (t *JobClassTagger) EventMatch(s string) bool {
	return strings.Contains(s, jobClassConfigDirMatch)
}

// EventCallback is triggered when the configuration directory changes.
// It reloads parameters and all rule files from the watched directory.
// FIXME: Only process the file that caused the event
func (t *JobClassTagger) EventCallback() {
	files, err := os.ReadDir(t.cfgPath)
	if err != nil {
		cclog.Fatal(err)
	}

	parametersFile := filepath.Join(t.cfgPath, parametersFileName)
	if util.CheckFileExists(parametersFile) {
		cclog.Info("Merge parameters")
		b, err := os.ReadFile(parametersFile)
		if err != nil {
			cclog.Warnf("prepareRule() > open file error: %v", err)
		}

		var paramTmp map[string]any
		if err := json.NewDecoder(bytes.NewReader(b)).Decode(&paramTmp); err != nil {
			cclog.Warn("Error while decoding parameters.json")
		}

		maps.Copy(t.parameters, paramTmp)
	}

	for _, fn := range files {
		fns := fn.Name()
		if fns != parametersFileName {
			cclog.Debugf("Process: %s", fns)
			filename := filepath.Join(t.cfgPath, fns)
			b, err := os.ReadFile(filename)
			if err != nil {
				cclog.Warnf("prepareRule() > open file error: %v", err)
				continue
			}
			t.prepareRule(b, fns)
		}
	}
}

func (t *JobClassTagger) initParameters() error {
	cclog.Info("Initialize parameters")
	parametersFile := filepath.Join(t.cfgPath, parametersFileName)
	b, err := os.ReadFile(parametersFile)
	if err != nil {
		cclog.Warnf("prepareRule() > open file error: %v", err)
		return err
	}

	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&t.parameters); err != nil {
		cclog.Warn("Error while decoding parameters.json")
		return err
	}

	return nil
}

// Register initializes the JobClassTagger by loading parameters and classification rules from external folder.
// It sets up a file watch on ./var/tagger/jobclasses if it exists, allowing for
// dynamic configuration updates without restarting the application.
// Returns an error if the configuration path does not exist or cannot be read.
func (t *JobClassTagger) Register() error {
	if t.cfgPath == "" {
		t.cfgPath = defaultJobClassConfigPath
	}
	t.tagType = tagTypeJobClass
	t.rules = make(map[string]ruleInfo)

	if !util.CheckFileExists(t.cfgPath) {
		return fmt.Errorf("configuration path does not exist: %s", t.cfgPath)
	}

	err := t.initParameters()
	if err != nil {
		cclog.Warnf("error reading parameters.json: %v", err)
		return err
	}

	files, err := os.ReadDir(t.cfgPath)
	if err != nil {
		return fmt.Errorf("error reading jobclasses folder: %#v", err)
	}

	for _, fn := range files {
		fns := fn.Name()
		if fns != parametersFileName {
			cclog.Infof("Process: %s", fns)
			filename := filepath.Join(t.cfgPath, fns)

			b, err := os.ReadFile(filename)
			if err != nil {
				cclog.Warnf("prepareRule() > open file error: %v", err)
				continue
			}
			t.prepareRule(b, fns)
		}
	}

	cclog.Infof("Setup file watch for %s", t.cfgPath)
	util.AddListener(t.cfgPath, t)

	t.repo = repository.GetJobRepository()
	t.getStatistics = archive.GetStatistics
	t.getMetricConfig = archive.GetMetricConfigSubCluster

	return nil
}

// Match evaluates all classification rules against a job and applies matching tags.
// It retrieves job statistics and metric configurations, then tests each rule's requirements
// and main expression. For each matching rule, it:
//   - Applies the classification tag to the job
//   - Generates and stores a hint message based on the rule's template
//
// The function constructs an evaluation environment containing:
//   - Job properties (duration, cores, nodes, state, etc.)
//   - Metric statistics (min, max, avg) and their configured limits
//   - Shared parameters defined in parameters.json
//   - Computed variables from the rule definition
//
// Rules are evaluated in arbitrary order. If multiple rules match, only the first
// encountered match is applied (FIXME: this should handle multiple matches).
func (t *JobClassTagger) Match(job *schema.Job) {
	jobStats, err := t.getStatistics(job)
	metricsList := t.getMetricConfig(job.Cluster, job.SubCluster)
	cclog.Infof("Enter  match rule with %d rules for job %d", len(t.rules), job.JobID)
	if err != nil {
		cclog.Errorf("job classification failed for job  %d: %#v", job.JobID, err)
		return
	}

	for tag, ri := range t.rules {
		env := make(map[string]any)
		maps.Copy(env, ri.env)
		cclog.Infof("Try to match rule %s for job %d", tag, job.JobID)

		// Initialize environment
		env["job"] = map[string]any{
			"shared":   job.Shared,
			"duration": job.Duration,
			"numCores": job.NumHWThreads,
			"numNodes": job.NumNodes,
			"jobState": job.State,
			"numAcc":   job.NumAcc,
			"smt":      job.SMT,
		}

		// add metrics to env
		for _, m := range ri.metrics {
			stats, ok := jobStats[m]
			if !ok {
				cclog.Errorf("job classification failed for job %d: missing metric '%s'", job.JobID, m)
				return
			}
			env[m] = map[string]any{
				"min": stats.Min,
				"max": stats.Max,
				"avg": stats.Avg,
				"limits": map[string]float64{
					"peak":    metricsList[m].Peak,
					"normal":  metricsList[m].Normal,
					"caution": metricsList[m].Caution,
					"alert":   metricsList[m].Alert,
				},
			}
		}

		// check rule requirements apply
		for _, r := range ri.requirements {
			ok, err := expr.Run(r, env)
			if err != nil {
				cclog.Errorf("error running requirement for rule %s: %#v", tag, err)
				return
			}
			if !ok.(bool) {
				cclog.Infof("requirement for rule %s not met", tag)
				return
			}
		}

		// validate rule expression
		for _, v := range ri.variables {
			value, err := expr.Run(v.expr, env)
			if err != nil {
				cclog.Errorf("error running rule %s: %#v", tag, err)
				return
			}
			env[v.name] = value
		}

		// dump.P(env)

		match, err := expr.Run(ri.rule, env)
		if err != nil {
			cclog.Errorf("error running rule %s: %#v", tag, err)
			return
		}
		if match.(bool) {
			cclog.Info("Rule matches!")
			id := *job.ID
			if !t.repo.HasTag(id, t.tagType, tag) {
				_, err := t.repo.AddTagOrCreateDirect(id, t.tagType, tag)
				if err != nil {
					return
				}
			}

			// process hint template
			var msg bytes.Buffer
			if err := ri.hint.Execute(&msg, env); err != nil {
				cclog.Errorf("Template error: %s", err.Error())
				return
			}

			// FIXME: Handle case where multiple tags apply
			// FIXME: Handle case where multiple tags apply
			err = t.repo.UpdateMetadata(job, "message", msg.String())
			if err != nil {
				return
			}
		} else {
			cclog.Info("Rule does not match!")
		}
	}
}
