// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"text/template"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/util"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/gookit/goutil/dump"
)

//go:embed jobclasses/*
var jobclassFiles embed.FS

type Variable struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
}

type ruleVariable struct {
	name string
	expr *vm.Program
}

type RuleFormat struct {
	Name         string     `json:"name"`
	Tag          string     `json:"tag"`
	Parameters   []string   `json:"parameters"`
	Metrics      []string   `json:"metrics"`
	Requirements []string   `json:"requirements"`
	Variables    []Variable `json:"variables"`
	Rule         string     `json:"rule"`
	Hint         string     `json:"hint"`
}

type ruleInfo struct {
	env          map[string]any
	metrics      []string
	requirements []*vm.Program
	variables    []ruleVariable
	rule         *vm.Program
	hint         *template.Template
}

type JobClassTagger struct {
	rules      map[string]ruleInfo
	parameters map[string]any
	tagType    string
	cfgPath    string
}

func (t *JobClassTagger) prepareRule(b []byte, fns string) {
	var rule RuleFormat
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&rule); err != nil {
		log.Warn("Error while decoding raw job meta json")
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
			log.Warnf("prepareRule() > missing parameter %s in rule %s", p, fns)
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
			log.Errorf("error compiling requirement %s: %#v", r, err)
			return
		}
		ri.requirements = append(ri.requirements, req)
	}

	// compile variables
	for _, v := range rule.Variables {
		req, err := expr.Compile(v.Expr, expr.AsFloat64())
		if err != nil {
			log.Errorf("error compiling requirement %s: %#v", v.Name, err)
			return
		}
		ri.variables = append(ri.variables, ruleVariable{name: v.Name, expr: req})
	}

	// compile rule
	exp, err := expr.Compile(rule.Rule, expr.AsBool())
	if err != nil {
		log.Errorf("error compiling rule %s: %#v", fns, err)
		return
	}
	ri.rule = exp

	// prepare hint template
	ri.hint, err = template.New(fns).Parse(rule.Hint)
	if err != nil {
		log.Errorf("error processing template %s: %#v", fns, err)
	}
	log.Infof("prepareRule() > processing %s with %d requirements and %d variables", fns, len(ri.requirements), len(ri.variables))

	delete(t.rules, rule.Tag)
	t.rules[rule.Tag] = ri
}

func (t *JobClassTagger) EventMatch(s string) bool {
	return strings.Contains(s, "jobclasses")
}

// FIXME: Only process the file that caused the event
func (t *JobClassTagger) EventCallback() {
	files, err := os.ReadDir(t.cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	for _, fn := range files {
		fns := fn.Name()
		log.Debugf("Process: %s", fns)
		filename := fmt.Sprintf("%s/%s", t.cfgPath, fns)
		b, err := os.ReadFile(filename)
		if err != nil {
			log.Warnf("prepareRule() > open file error: %v", err)
			return
		}
		t.prepareRule(b, fns)
	}
}

func (t *JobClassTagger) initParameters() error {
	log.Info("Initialize parameters")
	b, err := jobclassFiles.ReadFile("jobclasses/parameters.json")
	if err != nil {
		log.Warnf("prepareRule() > open file error: %v", err)
		return err
	}

	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&t.parameters); err != nil {
		log.Warn("Error while decoding parameters.json")
		return err
	}

	return nil
}

func (t *JobClassTagger) Register() error {
	t.cfgPath = "./var/tagger/jobclasses"
	t.tagType = "jobClass"

	err := t.initParameters()
	if err != nil {
		log.Warnf("error reading parameters.json: %v", err)
		return err
	}

	files, err := jobclassFiles.ReadDir("jobclasses")
	if err != nil {
		return fmt.Errorf("error reading app folder: %#v", err)
	}
	t.rules = make(map[string]ruleInfo, 0)
	for _, fn := range files {
		fns := fn.Name()
		if fns != "parameters.json" {
			filename := fmt.Sprintf("jobclasses/%s", fns)
			log.Infof("Process: %s", fns)

			b, err := jobclassFiles.ReadFile(filename)
			if err != nil {
				log.Warnf("prepareRule() > open file error: %v", err)
				return err
			}
			t.prepareRule(b, fns)
		}
	}

	if util.CheckFileExists(t.cfgPath) {
		t.EventCallback()
		log.Infof("Setup file watch for %s", t.cfgPath)
		util.AddListener(t.cfgPath, t)
	}

	return nil
}

func (t *JobClassTagger) Match(job *schema.Job) {
	r := repository.GetJobRepository()
	jobstats, err := archive.GetStatistics(job)
	log.Infof("Enter  match rule with %d rules for job %d", len(t.rules), job.JobID)
	if err != nil {
		log.Errorf("job classification failed for job  %d: %#v", job.JobID, err)
		return
	}

	for tag, ri := range t.rules {
		env := make(map[string]any)
		maps.Copy(env, ri.env)
		log.Infof("Try to match rule %s for job %d", tag, job.JobID)
		env["job"] = map[string]any{
			"exclusive": job.Exclusive,
			"duration":  job.Duration,
			"numCores":  job.NumHWThreads,
			"numNodes":  job.NumNodes,
			"jobState":  job.State,
			"numAcc":    job.NumAcc,
			"smt":       job.SMT,
		}

		// add metrics to env
		for _, m := range ri.metrics {
			stats, ok := jobstats[m]
			if !ok {
				log.Errorf("job classification failed for job %d: missing metric '%s'", job.JobID, m)
				return
			}
			env[m] = stats.Avg
		}

		// check rule requirements apply
		for _, r := range ri.requirements {
			ok, err := expr.Run(r, env)
			if err != nil {
				log.Errorf("error running requirement for rule %s: %#v", tag, err)
				return
			}
			if !ok.(bool) {
				log.Infof("requirement for rule %s not met", tag)
				return
			}
		}

		// validate rule expression
		for _, v := range ri.variables {
			value, err := expr.Run(v.expr, env)
			if err != nil {
				log.Errorf("error running rule %s: %#v", tag, err)
				return
			}
			env[v.name] = value
		}

		dump.P(env)

		match, err := expr.Run(ri.rule, env)
		if err != nil {
			log.Errorf("error running rule %s: %#v", tag, err)
			return
		}
		if match.(bool) {
			log.Info("Rule matches!")
			id := job.ID
			if !r.HasTag(id, t.tagType, tag) {
				r.AddTagOrCreateDirect(id, t.tagType, tag)
			}

			// process hint template
			var msg bytes.Buffer
			if err := ri.hint.Execute(&msg, env); err != nil {
				log.Errorf("Template error: %s", err.Error())
				return
			}

			// FIXME: Handle case where multiple tags apply
			r.UpdateMetadata(job, "message", msg.String())
		} else {
			log.Info("Rule does not match!")
		}
	}
}
