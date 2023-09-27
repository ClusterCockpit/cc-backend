// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package config

import (
	"bytes"
	"encoding/json"
	"log"
	"os"

	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

var Keys schema.ProgramConfig = schema.ProgramConfig{
	Addr:                      "localhost:8080",
	DisableAuthentication:     false,
	EmbedStaticFiles:          true,
	DBDriver:                  "sqlite3",
	DB:                        "./var/job.db",
	Archive:                   json.RawMessage(`{\"kind\":\"file\",\"path\":\"./var/job-archive\"}`),
	DisableArchive:            false,
	Validate:                  false,
	SessionMaxAge:             "168h",
	StopJobsExceedingWalltime: 0,
	ShortRunningJobsDuration:  5 * 60,
	UiDefaults: map[string]interface{}{
		"analysis_view_histogramMetrics":         []string{"flops_any", "mem_bw", "mem_used"},
		"analysis_view_scatterPlotMetrics":       [][]string{{"flops_any", "mem_bw"}, {"flops_any", "cpu_load"}, {"cpu_load", "mem_bw"}},
		"job_view_nodestats_selectedMetrics":     []string{"flops_any", "mem_bw", "mem_used"},
		"job_view_polarPlotMetrics":              []string{"flops_any", "mem_bw", "mem_used"},
		"job_view_selectedMetrics":               []string{"flops_any", "mem_bw", "mem_used"},
		"plot_general_colorBackground":           true,
		"plot_general_colorscheme":               []string{"#00bfff", "#0000ff", "#ff00ff", "#ff0000", "#ff8000", "#ffff00", "#80ff00"},
		"plot_general_lineWidth":                 3,
		"plot_list_jobsPerPage":                  50,
		"plot_list_selectedMetrics":              []string{"cpu_load", "mem_used", "flops_any", "mem_bw"},
		"plot_view_plotsPerRow":                  3,
		"plot_view_showPolarplot":                true,
		"plot_view_showRoofline":                 true,
		"plot_view_showStatTable":                true,
		"system_view_selectedMetric":             "cpu_load",
		"analysis_view_selectedTopEntity":        "user",
		"analysis_view_selectedTopCategory":      "totalWalltime",
		"status_view_selectedTopUserCategory":    "totalJobs",
		"status_view_selectedTopProjectCategory": "totalJobs",
	},
}

func Init(flagConfigFile string) {
	raw, err := os.ReadFile(flagConfigFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatalf("CONFIG ERROR: %v", err)
		}
	} else {
		if err := schema.Validate(schema.Config, bytes.NewReader(raw)); err != nil {
			log.Fatalf("Validate config: %v\n", err)
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&Keys); err != nil {
			log.Fatalf("could not decode: %v", err)
		}

		if Keys.Clusters == nil || len(Keys.Clusters) < 1 {
			log.Fatal("At least one cluster required in config!")
		}
	}
}
