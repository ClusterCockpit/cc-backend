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
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/util"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

const (
	tagType = "app"
	appPath = "./var/tagger/apps"
)

//go:embed apps/*
var appFiles embed.FS

type appInfo struct {
	tag     string
	strings []string
}

type AppTagger struct {
	apps map[string]appInfo
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

func (t *AppTagger) EventCallback() {
	files, err := os.ReadDir(appPath)
	if err != nil {
		log.Fatal(err)
	}

	for _, fn := range files {
		fns := fn.Name()
		log.Debugf("Process: %s", fns)
		f, err := os.Open(fmt.Sprintf("%s/%s", appPath, fns))
		if err != nil {
			log.Errorf("error opening app file %s: %#v", fns, err)
		}
		t.scanApp(f, fns)
	}
}

func (t *AppTagger) Register() error {
	files, err := appFiles.ReadDir("apps")
	if err != nil {
		return fmt.Errorf("error reading app folder: %#v", err)
	}
	t.apps = make(map[string]appInfo, 0)
	for _, fn := range files {
		fns := fn.Name()
		log.Debugf("Process: %s", fns)
		f, err := appFiles.Open(fmt.Sprintf("apps/%s", fns))
		if err != nil {
			return fmt.Errorf("error opening app file %s: %#v", fns, err)
		}
		t.scanApp(f, fns)
	}

	if util.CheckFileExists(appPath) {
		log.Infof("Setup file watch for %s", appPath)
		util.AddListener(appPath, t)
	}

	return nil
}

func (t *AppTagger) Match(job *schema.Job) {
	r := repository.GetJobRepository()
	meta, err := r.FetchMetadata(job)
	if err != nil {
		log.Error("cannot fetch meta data")
	}
	jobscript, ok := meta["jobScript"]
	if ok {
		id := job.ID

	out:
		for _, a := range t.apps {
			tag := a.tag
			for _, s := range a.strings {
				if strings.Contains(jobscript, s) {
					if !r.HasTag(id, tagType, tag) {
						r.AddTagOrCreateDirect(id, tagType, tag)
						break out
					}
				}
			}
		}
	} else {
		log.Infof("Cannot extract job script for job: %d on %s", job.JobID, job.Cluster)
	}
}
