// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package config

import (
	"encoding/json"
	"log"
	"os"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
)

type Cluster struct {
	Name                 string              `json:"name"`
	FilterRanges         *model.FilterRanges `json:"filterRanges"`
	MetricDataRepository json.RawMessage     `json:"metricDataRepository"`
}

// Format of the configuration (file). See below for the defaults.
type ProgramConfig struct {
	// Address where the http (or https) server will listen on (for example: 'localhost:80').
	Addr string `json:"addr"`

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

	// 'sqlite3' or 'mysql' (mysql will work for mariadb as well)
	DBDriver string `json:"db-driver"`

	// For sqlite3 a filename, for mysql a DSN in this format: https://github.com/go-sql-driver/mysql#dsn-data-source-name (Without query parameters!).
	DB string `json:"db"`

	// Config for job archive
	Archive json.RawMessage `json:"archive"`

	// Keep all metric data in the metric data repositories,
	// do not write to the job-archive.
	DisableArchive bool `json:"disable-archive"`

	// For LDAP Authentication and user synchronisation.
	LdapConfig *auth.LdapConfig    `json:"ldap"`
	JwtConfig  *auth.JWTAuthConfig `json:"jwts"`

	// If 0 or empty, the session/token does not expire!
	SessionMaxAge string `json:"session-max-age"`

	// If both those options are not empty, use HTTPS using those certificates.
	HttpsCertFile string `json:"https-cert-file"`
	HttpsKeyFile  string `json:"https-key-file"`

	// If not the empty string and `addr` does not end in ":80",
	// redirect every request incoming at port 80 to that url.
	RedirectHttpTo string `json:"redirect-http-to"`

	// If overwriten, at least all the options in the defaults below must
	// be provided! Most options here can be overwritten by the user.
	UiDefaults map[string]interface{} `json:"ui-defaults"`

	// Where to store MachineState files
	MachineStateDir string `json:"machine-state-dir"`

	// If not zero, automatically mark jobs as stopped running X seconds longer than their walltime.
	StopJobsExceedingWalltime int `json:"stop-jobs-exceeding-walltime"`

	// Array of Clusters
	Clusters []*Cluster `json:"Clusters"`
}

var Keys ProgramConfig = ProgramConfig{
	Addr:                  ":8080",
	DisableAuthentication: false,
	EmbedStaticFiles:      true,
	DBDriver:              "sqlite3",
	DB:                    "./var/job.db",
	Archive:               []byte(`{\"kind\":\"file\",\"path\":\"./var/job-archive\"}`),
	DisableArchive:        false,
	LdapConfig:            nil,
	SessionMaxAge:         "168h",
	UiDefaults: map[string]interface{}{
		"analysis_view_histogramMetrics":     []string{"flops_any", "mem_bw", "mem_used"},
		"analysis_view_scatterPlotMetrics":   [][]string{{"flops_any", "mem_bw"}, {"flops_any", "cpu_load"}, {"cpu_load", "mem_bw"}},
		"job_view_nodestats_selectedMetrics": []string{"flops_any", "mem_bw", "mem_used"},
		"job_view_polarPlotMetrics":          []string{"flops_any", "mem_bw", "mem_used", "net_bw", "file_bw"},
		"job_view_selectedMetrics":           []string{"flops_any", "mem_bw", "mem_used"},
		"plot_general_colorBackground":       true,
		"plot_general_colorscheme":           []string{"#00bfff", "#0000ff", "#ff00ff", "#ff0000", "#ff8000", "#ffff00", "#80ff00"},
		"plot_general_lineWidth":             3,
		"plot_list_hideShortRunningJobs":     5 * 60,
		"plot_list_jobsPerPage":              50,
		"plot_list_selectedMetrics":          []string{"cpu_load", "ipc", "mem_used", "flops_any", "mem_bw"},
		"plot_view_plotsPerRow":              3,
		"plot_view_showPolarplot":            true,
		"plot_view_showRoofline":             true,
		"plot_view_showStatTable":            true,
		"system_view_selectedMetric":         "cpu_load",
	},
	StopJobsExceedingWalltime: 0,
}

func Init(flagConfigFile string) {
	f, err := os.Open(flagConfigFile)
	if err != nil {
		if !os.IsNotExist(err) || flagConfigFile != "./config.json" {
			log.Fatal(err)
		}
	} else {
		dec := json.NewDecoder(f)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&Keys); err != nil {
			log.Fatal(err)
		}
		f.Close()
	}
}
