// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/diegoholiveira/jsonlogic"
)

const tagType = "app"

//go:embed apps/*
var appFiles embed.FS

type appInfo struct {
	tag     string
	strings []string
}

type AppTagger struct {
	apps     []appInfo
	jsonRule map[string]interface{} // Store a single JSON rule
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

// LoadRule function to load JSON logic rule from rules.json
func (t *AppTagger) LoadRule() error {
	file, err := os.Open("rules.json")
	if err != nil {
		return fmt.Errorf("error opening rules file: %v", err)
	}
	defer file.Close()

	var rule map[string]interface{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&rule); err != nil {
		return fmt.Errorf("error decoding rule: %v", err)
	}
	t.jsonRule = rule
	return nil
}

func (t *AppTagger) Match(job *schema.Job) {
	r := repository.GetJobRepository()
	meta, err := r.FetchMetadata(job)
	if err != nil {
		log.Error("cannot fetch meta data")
		return
	}

	// Prepare the data for JSON logic evaluation
	data := map[string]interface{}{
		"metaData":   meta,
		"statistics": job.Statistics,
	}

	jobscript, ok := meta["jobScript"]
	if ok {
		id := job.ID

	out:
		// Apply JSON logic rule
		if t.jsonRule != nil {
			result, err := jsonlogic.Apply(t.jsonRule, data)
			if err != nil {
				log.Errorf("error applying JSON logic: %v", err)
			} else if match, ok := result.(bool); ok && match {
				tag := "detectedApp" // Define a tag for detected apps
				if !r.HasTag(id, tagType, tag) {
					r.AddTagOrCreate(id, tagType, tag)
					break out
				}
			}
		}

		// Original app matching logic remains for backward compatibility (if needed).
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

func main() {
	appTagger := &AppTagger{}

	// Load the JSON logic rule
	if err := appTagger.LoadRule(); err != nil {
		log.Fatalf("Failed to load rules: %v", err)
	}

	// Assume you have a job to process
	var job *schema.Job
	// Fetch or define the job data here

	// Apply the matching logic
	appTagger.Match(job)
}

//
//why are we using json logics, cannot we simply use read input and apply if/else logic to it??
//the first part is to give the json logic an input

//making rules for taggingthe data
