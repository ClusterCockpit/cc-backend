// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package tagger

import (
	"bufio"
	"embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

const tagType = "app"

//go:embed apps/*
var appFiles embed.FS

type appInfo struct {
	tag     string
	strings []string
}

type AppTagger struct {
	apps []appInfo
}

func (t *AppTagger) Register() error {
	files, err := appFiles.ReadDir("apps")
	if err != nil {
		return fmt.Errorf("error reading app folder: %#v", err)
	}
	t.apps = make([]appInfo, 0)

	for _, fn := range files {
		fns := fn.Name()
		log.Debugf("Process: %s", fns)
		f, err := appFiles.Open(fmt.Sprintf("apps/%s", fns))
		if err != nil {
			return fmt.Errorf("error opening app file %s: %#v", fns, err)
		}
		scanner := bufio.NewScanner(f)
		ai := appInfo{tag: strings.TrimSuffix(fns, filepath.Ext(fns)), strings: make([]string, 0)}

		for scanner.Scan() {
			ai.strings = append(ai.strings, scanner.Text())
		}
		t.apps = append(t.apps, ai)
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
						r.AddTagOrCreate(id, tagType, tag)
						break out
					}
				}
			}
		}
	} else {
		log.Infof("Cannot extract job script for job: %d on %s", job.JobID, job.Cluster)
	}
}
