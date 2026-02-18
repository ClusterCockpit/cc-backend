// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package config implements the program configuration data structures, validation and parsing
package config

import (
	"bytes"
	"encoding/json"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/resampler"
)

type ProgramConfig struct {
	// Address where the http (or https) server will listen on (for example: 'localhost:80').
	Addr string `json:"addr"`

	// Addresses from which secured admin API endpoints can be reached, can be wildcard "*"
	APIAllowedIPs []string `json:"api-allowed-ips"`

	APISubjects *NATSConfig `json:"api-subjects"`

	// Drop root permissions once .env was read and the port was taken.
	User  string `json:"user"`
	Group string `json:"group"`

	// Disable authentication (for everything: API, Web-UI, ...)
	DisableAuthentication bool `json:"disable-authentication"`

	// If `embed-static-files` is true (default), the frontend files are directly
	// embeded into the go binary and expected to be in web/frontend. Only if
	// it is false the files in `static-files` are served instead.
	EmbedStaticFiles bool   `json:"embed-static-files"`
	StaticFiles      string `json:"static-files"`

	// Path to SQLite database file
	DB string `json:"db"`

	EnableJobTaggers bool `json:"enable-job-taggers"`

	// Validate json input against schema
	Validate bool `json:"validate"`

	// If 0 or empty, the session does not expire!
	SessionMaxAge string `json:"session-max-age"`

	// If both those options are not empty, use HTTPS using those certificates.
	HTTPSCertFile string `json:"https-cert-file"`
	HTTPSKeyFile  string `json:"https-key-file"`

	// If not the empty string and `addr` does not end in ":80",
	// redirect every request incoming at port 80 to that url.
	RedirectHTTPTo string `json:"redirect-http-to"`

	// Where to store MachineState files
	MachineStateDir string `json:"machine-state-dir"`

	// If not zero, automatically mark jobs as stopped running X seconds longer than their walltime.
	StopJobsExceedingWalltime int `json:"stop-jobs-exceeding-walltime"`

	// Defines time X in seconds in which jobs are considered to be "short" and will be filtered in specific views.
	ShortRunningJobsDuration int `json:"short-running-jobs-duration"`

	// Energy Mix CO2 Emission Constant [g/kWh]
	// If entered, displays estimated CO2 emission for job based on jobs totalEnergy
	EmissionConstant int `json:"emission-constant"`

	// If exists, will enable dynamic zoom in frontend metric plots using the configured values
	EnableResampling *ResampleConfig `json:"resampling"`

	// Systemd unit name for log viewer (default: "clustercockpit")
	SystemdUnit string `json:"systemd-unit"`

	// Node state retention configuration
	NodeStateRetention *NodeStateRetention `json:"nodestate-retention"`
}

type NodeStateRetention struct {
	Policy             string `json:"policy"`      // "delete" or "move"
	Age                int    `json:"age"`         // hours, default 24
	TargetKind         string `json:"target-kind"` // "file" or "s3"
	TargetPath         string `json:"target-path"`
	TargetEndpoint     string `json:"target-endpoint"`
	TargetBucket       string `json:"target-bucket"`
	TargetAccessKey    string `json:"target-access-key"`
	TargetSecretKey    string `json:"target-secret-key"`
	TargetRegion       string `json:"target-region"`
	TargetUsePathStyle bool   `json:"target-use-path-style"`
	MaxFileSizeMB      int    `json:"max-file-size-mb"`
}

type ResampleConfig struct {
	// Minimum number of points to trigger resampling of data
	MinimumPoints int `json:"minimum-points"`
	// Array of resampling target resolutions, in seconds; Example: [600,300,60]
	Resolutions []int `json:"resolutions"`
	// Trigger next zoom level at less than this many visible datapoints
	Trigger int `json:"trigger"`
}

type NATSConfig struct {
	SubjectJobEvent  string `json:"subject-job-event"`
	SubjectNodeState string `json:"subject-node-state"`
}

type IntRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type TimeRange struct {
	From  *time.Time `json:"from"`
	To    *time.Time `json:"to"`
	Range string     `json:"range,omitempty"`
}

type FilterRanges struct {
	Duration  *IntRange  `json:"duration"`
	NumNodes  *IntRange  `json:"num-nodes"`
	StartTime *TimeRange `json:"start-time"`
}

var Keys ProgramConfig = ProgramConfig{
	Addr:                      "localhost:8080",
	EmbedStaticFiles:          true,
	DB:                        "./var/job.db",
	SessionMaxAge:             "168h",
	StopJobsExceedingWalltime: 0,
	ShortRunningJobsDuration:  5 * 60,
}

func Init(mainConfig json.RawMessage) {
	Validate(configSchema, mainConfig)
	dec := json.NewDecoder(bytes.NewReader(mainConfig))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&Keys); err != nil {
		cclog.Abortf("Config Init: Could not decode config file '%s'.\nError: %s\n", mainConfig, err.Error())
	}

	if Keys.EnableResampling != nil && Keys.EnableResampling.MinimumPoints > 0 {
		resampler.SetMinimumRequiredPoints(Keys.EnableResampling.MinimumPoints)
	}
}
