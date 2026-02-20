// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package tagger

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
)

func metadataKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

const (
	// defaultConfigPath is the default path for application tagging configuration
	defaultConfigPath = "./var/tagger/apps"
	// tagTypeApp is the tag type identifier for application tags
	tagTypeApp = "app"
	// configDirMatch is the directory name used for matching filesystem events
	configDirMatch = "apps"
)

type appInfo struct {
	tag      string
	patterns []*regexp.Regexp
}

// AppTagger detects applications by matching patterns in job scripts.
// It loads application patterns from an external configuration directory and can dynamically reload
// configuration when files change. When a job script matches a pattern,
// the corresponding application tag is automatically applied.
type AppTagger struct {
	// apps holds application patterns in deterministic order
	apps []appInfo
	// tagType is the type of tag ("app")
	tagType string
	// cfgPath is the path to watch for configuration changes
	cfgPath string
}

func (t *AppTagger) scanApp(f *os.File, fns string) {
	scanner := bufio.NewScanner(f)
	tag := strings.TrimSuffix(fns, filepath.Ext(fns))
	ai := appInfo{tag: tag, patterns: make([]*regexp.Regexp, 0)}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		re, err := regexp.Compile(line)
		if err != nil {
			cclog.Errorf("invalid regex pattern '%s' in %s: %v", line, fns, err)
			continue
		}
		ai.patterns = append(ai.patterns, re)
	}

	// Remove existing entry for this tag if present
	for i, a := range t.apps {
		if a.tag == tag {
			t.apps = append(t.apps[:i], t.apps[i+1:]...)
			break
		}
	}

	cclog.Infof("AppTagger loaded %d patterns for %s", len(ai.patterns), tag)
	t.apps = append(t.apps, ai)
}

// EventMatch checks if a filesystem event should trigger configuration reload.
// It returns true if the event path contains "apps".
func (t *AppTagger) EventMatch(s string) bool {
	return strings.Contains(s, configDirMatch)
}

// EventCallback is triggered when the configuration directory changes.
// It reloads all application pattern files from the watched directory.
func (t *AppTagger) EventCallback() {
	files, err := os.ReadDir(t.cfgPath)
	if err != nil {
		cclog.Fatal(err)
	}

	for _, fn := range files {
		if fn.IsDir() {
			continue
		}
		fns := fn.Name()
		cclog.Debugf("Process: %s", fns)
		f, err := os.Open(filepath.Join(t.cfgPath, fns))
		if err != nil {
			cclog.Errorf("error opening app file %s: %#v", fns, err)
			continue
		}
		t.scanApp(f, fns)
		if err := f.Close(); err != nil {
			cclog.Errorf("error closing app file %s: %#v", fns, err)
		}
	}
}

// Register initializes the AppTagger by loading application patterns from external folder.
// It sets up a file watch on ./var/tagger/apps if it exists, allowing for
// dynamic configuration updates without restarting the application.
// Returns an error if the configuration path does not exist or cannot be read.
func (t *AppTagger) Register() error {
	if t.cfgPath == "" {
		t.cfgPath = defaultConfigPath
	}
	t.tagType = tagTypeApp
	t.apps = make([]appInfo, 0)

	if !util.CheckFileExists(t.cfgPath) {
		return fmt.Errorf("configuration path does not exist: %s", t.cfgPath)
	}

	files, err := os.ReadDir(t.cfgPath)
	if err != nil {
		return fmt.Errorf("error reading app folder: %#v", err)
	}

	for _, fn := range files {
		if fn.IsDir() {
			continue
		}
		fns := fn.Name()
		cclog.Debugf("Process: %s", fns)
		f, err := os.Open(filepath.Join(t.cfgPath, fns))
		if err != nil {
			cclog.Errorf("error opening app file %s: %#v", fns, err)
			continue
		}
		t.scanApp(f, fns)
		if err := f.Close(); err != nil {
			cclog.Errorf("error closing app file %s: %#v", fns, err)
		}
	}

	cclog.Infof("Setup file watch for %s", t.cfgPath)
	util.AddListener(t.cfgPath, t)

	return nil
}

// Match attempts to detect the application used by a job by analyzing its job script.
// It fetches the job metadata, extracts the job script, and matches it against
// all configured application patterns using regular expressions.
// If a match is found, the corresponding application tag is added to the job.
// Only the first matching application is tagged.
func (t *AppTagger) Match(job *schema.Job) {
	r := repository.GetJobRepository()

	if len(t.apps) == 0 {
		cclog.Warn("AppTagger: no app patterns loaded, skipping match")
		return
	}

	metadata, err := r.FetchMetadata(job)
	if err != nil {
		cclog.Infof("AppTagger: cannot fetch metadata for job %d on %s: %v", job.JobID, job.Cluster, err)
		return
	}

	if metadata == nil {
		cclog.Infof("AppTagger: metadata is nil for job %d on %s", job.JobID, job.Cluster)
		return
	}

	jobscript, ok := metadata["jobScript"]
	if !ok {
		cclog.Infof("AppTagger: no 'jobScript' key in metadata for job %d on %s (keys: %v)",
			job.JobID, job.Cluster, metadataKeys(metadata))
		return
	}

	if len(jobscript) == 0 {
		cclog.Infof("AppTagger: empty jobScript for job %d on %s", job.JobID, job.Cluster)
		return
	}

	id := *job.ID
	jobscriptLower := strings.ToLower(jobscript)
	cclog.Debugf("AppTagger: matching job %d (script length: %d) against %d apps", id, len(jobscriptLower), len(t.apps))

	for _, a := range t.apps {
		for _, re := range a.patterns {
			if re.MatchString(jobscriptLower) {
				if r.HasTag(id, t.tagType, a.tag) {
					cclog.Debugf("AppTagger: job %d already has tag %s:%s, skipping", id, t.tagType, a.tag)
				} else {
					cclog.Infof("AppTagger: pattern '%s' matched for app '%s' on job %d", re.String(), a.tag, id)
					if _, err := r.AddTagOrCreateDirect(id, t.tagType, a.tag); err != nil {
						cclog.Errorf("AppTagger: failed to add tag '%s' to job %d: %v", a.tag, id, err)
					}
				}
				return
			}
		}
	}

	cclog.Debugf("AppTagger: no pattern matched for job %d on %s", id, job.Cluster)
}
