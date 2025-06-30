// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"
)

//go:embed apps/*
var appFiles embed.FS

type appInfo struct {
	tag     string
	strings []string
}

type AppTagger struct {
	apps    map[string]appInfo
	tagType string
	cfgPath string
}

func (t *AppTagger) scanApp(f fs.File, fns string) {
	scanner := bufio.NewScanner(f)
	ai := appInfo{tag: strings.TrimSuffix(fns, filepath.Ext(fns)), strings: make([]string, 0)}

	for scanner.Scan() {
		ai.strings = append(ai.strings, scanner.Text())
	}
	delete(t.apps, ai.tag)
	t.apps[ai.tag] = ai
}

func (t *AppTagger) EventMatch(s string) bool {
	return strings.Contains(s, "apps")
}

// FIXME: Only process the file that caused the event
func (t *AppTagger) EventCallback() {
	files, err := os.ReadDir(t.cfgPath)
	if err != nil {
		cclog.Fatal(err)
	}

	for _, fn := range files {
		fns := fn.Name()
		cclog.Debugf("Process: %s", fns)
		f, err := os.Open(fmt.Sprintf("%s/%s", t.cfgPath, fns))
		if err != nil {
			cclog.Errorf("error opening app file %s: %#v", fns, err)
		}
		t.scanApp(f, fns)
	}
}

func (t *AppTagger) Register() error {
	t.cfgPath = "./var/tagger/apps"
	t.tagType = "app"

	files, err := appFiles.ReadDir("apps")
	if err != nil {
		return fmt.Errorf("error reading app folder: %#v", err)
	}
	t.apps = make(map[string]appInfo, 0)
	for _, fn := range files {
		fns := fn.Name()
		cclog.Debugf("Process: %s", fns)
		f, err := appFiles.Open(fmt.Sprintf("apps/%s", fns))
		if err != nil {
			return fmt.Errorf("error opening app file %s: %#v", fns, err)
		}
		defer f.Close()
		t.scanApp(f, fns)
	}

	if util.CheckFileExists(t.cfgPath) {
		t.EventCallback()
		cclog.Infof("Setup file watch for %s", t.cfgPath)
		util.AddListener(t.cfgPath, t)
	}

	return nil
}

func (t *AppTagger) Match(job *schema.Job) {
	r := repository.GetJobRepository()
	metadata, err := r.FetchMetadata(job)
	if err != nil {
		cclog.Infof("Cannot fetch metadata for job: %d on %s", job.JobID, job.Cluster)
		return
	}

	jobscript, ok := metadata["jobScript"]
	if ok {
		id := *job.ID

	out:
		for _, a := range t.apps {
			tag := a.tag
			for _, s := range a.strings {
				matched, _ := regexp.MatchString(s, strings.ToLower(jobscript))
				if matched {
					if !r.HasTag(id, t.tagType, tag) {
						r.AddTagOrCreateDirect(id, t.tagType, tag)
						break out
					}
				}
			}
		}
	} else {
		cclog.Infof("Cannot extract job script for job: %d on %s", job.JobID, job.Cluster)
	}
}
