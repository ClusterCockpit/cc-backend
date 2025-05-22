// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/util"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

//go:embed jobclasses/*
var jobclassFiles embed.FS

type ruleInfo struct {
	tag  string
	rule *vm.Program
}

type JobClassTagger struct {
	rules   map[string]ruleInfo
	tagType string
	cfgPath string
}

func (t *JobClassTagger) compileRule(f fs.File, fns string) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(f)
	if err != nil {
		log.Errorf("error reading rule file %s: %#v", fns, err)
	}
	prg, err := expr.Compile(buf.String(), expr.AsBool())
	if err != nil {
		log.Errorf("error compiling rule %s: %#v", fns, err)
	}
	ri := ruleInfo{tag: strings.TrimSuffix(fns, filepath.Ext(fns)), rule: prg}

	delete(t.rules, ri.tag)
	t.rules[ri.tag] = ri
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
		f, err := os.Open(fmt.Sprintf("%s/%s", t.cfgPath, fns))
		if err != nil {
			log.Errorf("error opening app file %s: %#v", fns, err)
		}
		t.compileRule(f, fns)
	}
}

func (t *JobClassTagger) Register() error {
	t.cfgPath = "./var/tagger/jobclasses"
	t.tagType = "jobClass"

	files, err := appFiles.ReadDir("jobclasses")
	if err != nil {
		return fmt.Errorf("error reading app folder: %#v", err)
	}
	t.rules = make(map[string]ruleInfo, 0)
	for _, fn := range files {
		fns := fn.Name()
		log.Debugf("Process: %s", fns)
		f, err := appFiles.Open(fmt.Sprintf("apps/%s", fns))
		if err != nil {
			return fmt.Errorf("error opening app file %s: %#v", fns, err)
		}
		defer f.Close()
		t.compileRule(f, fns)
	}

	if util.CheckFileExists(t.cfgPath) {
		t.EventCallback()
		log.Infof("Setup file watch for %s", t.cfgPath)
		util.AddListener(t.cfgPath, t)
	}

	return nil
}

func (t *JobClassTagger) Match(job *schema.JobMeta) {
	r := repository.GetJobRepository()

	for _, ri := range t.rules {
		tag := ri.tag
		output, err := expr.Run(ri.rule, job)
		if err != nil {
			log.Errorf("error running rule %s: %#v", tag, err)
		}
		if output.(bool) {
			id := job.ID
			if !r.HasTag(*id, t.tagType, tag) {
				r.AddTagOrCreateDirect(*id, t.tagType, tag)
			}
		}
	}
}
