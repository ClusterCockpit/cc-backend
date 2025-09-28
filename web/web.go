// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package web implements the HTML templating and web frontend configuration
package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"
)

type WebConfig struct {
	JobList           JobListConfig     `json:"jobList"`
	NodeList          NodeListConfig    `json:"nodeList"`
	JobView           JobViewConfig     `json:"jobView"`
	MetricConfig      MetricConfig      `json:"metricConfig"`
	PlotConfiguration PlotConfiguration `json:"plotConfiguration"`
}

type JobListConfig struct {
	UsePaging     bool `json:"usePaging"`
	ShowFootprint bool `json:"showFootprint"`
}

type NodeListConfig struct {
	UsePaging bool `json:"usePaging"`
}

type JobViewConfig struct {
	ShowPolarPlot bool `json:"showPolarPlot"`
	ShowFootprint bool `json:"showFootprint"`
	ShowRoofline  bool `json:"showRoofline"`
	ShowStatTable bool `json:"showStatTable"`
}

type MetricConfig struct {
	JobListMetrics      []string        `json:"jobListMetrics"`
	JobViewPlotMetrics  []string        `json:"jobViewPlotMetrics"`
	JobViewTableMetrics []string        `json:"jobViewTableMetrics"`
	Clusters            []ClusterConfig `json:"clusters"`
}

type ClusterConfig struct {
	Name                string             `json:"name"`
	JobListMetrics      []string           `json:"jobListMetrics"`
	JobViewPlotMetrics  []string           `json:"jobViewPlotMetrics"`
	JobViewTableMetrics []string           `json:"jobViewTableMetrics"`
	SubClusters         []SubClusterConfig `json:"subClusters"`
}

type SubClusterConfig struct {
	Name                string   `json:"name"`
	JobListMetrics      []string `json:"jobListMetrics"`
	JobViewPlotMetrics  []string `json:"jobViewPlotMetrics"`
	JobViewTableMetrics []string `json:"jobViewTableMetrics"`
}

type PlotConfiguration struct {
	ColorBackground bool     `json:"colorBackground"`
	PlotsPerRow     int      `json:"plotsPerRow"`
	LineWidth       int      `json:"lineWidth"`
	ColorScheme     []string `json:"colorScheme"`
}

var initOnce sync.Once

var UIDefaults = WebConfig{
	JobList: JobListConfig{
		UsePaging:     false,
		ShowFootprint: true,
	},
	NodeList: NodeListConfig{
		UsePaging: true,
	},
	JobView: JobViewConfig{
		ShowPolarPlot: true,
		ShowFootprint: true,
		ShowRoofline:  true,
		ShowStatTable: true,
	},
	MetricConfig: MetricConfig{
		JobListMetrics:      []string{"flops_any", "mem_bw", "mem_used"},
		JobViewPlotMetrics:  []string{"flops_any", "mem_bw", "mem_used"},
		JobViewTableMetrics: []string{"flops_any", "mem_bw", "mem_used"},
	},
	PlotConfiguration: PlotConfiguration{
		ColorBackground: true,
		PlotsPerRow:     3,
		LineWidth:       3,
		ColorScheme:     []string{"#00bfff", "#0000ff", "#ff00ff", "#ff0000", "#ff8000", "#ffff00", "#80ff00"},
	},
}

//
// 	map[string]any{
// 	"analysis_view_histogramMetrics":         []string{"flops_any", "mem_bw", "mem_used"},
// 	"analysis_view_scatterPlotMetrics":       [][]string{{"flops_any", "mem_bw"}, {"flops_any", "cpu_load"}, {"cpu_load", "mem_bw"}},
// 	"job_view_nodestats_selectedMetrics":     []string{"flops_any", "mem_bw", "mem_used"},
// 	"plot_list_jobsPerPage":                  50,
// 	"system_view_selectedMetric":             "cpu_load",
// 	"analysis_view_selectedTopEntity":        "user",
// 	"analysis_view_selectedTopCategory":      "totalWalltime",
// 	"status_view_selectedTopUserCategory":    "totalJobs",
// 	"status_view_selectedTopProjectCategory": "totalJobs",
// }

func Init(rawConfig json.RawMessage, disableArchive bool) error {
	var err error

	initOnce.Do(func() {
		config.Validate(configSchema, rawConfig)
		if err = json.Unmarshal(rawConfig, &UIDefaults); err != nil {
			cclog.Warn("Error while unmarshaling raw config json")
			return
		}
	})

	return err
}

// / Go's embed is only allowed to embed files in a subdirectory of the embedding package ([see here](https://github.com/golang/go/issues/46056)).
//
//go:embed frontend/public/*
var frontendFiles embed.FS

func ServeFiles() http.Handler {
	publicFiles, err := fs.Sub(frontendFiles, "frontend/public")
	if err != nil {
		cclog.Abortf("Serve Files: Could not find 'frontend/public' file directory.\nError: %s\n", err.Error())
	}
	return http.FileServer(http.FS(publicFiles))
}

//go:embed templates/*
var templateFiles embed.FS

var templates map[string]*template.Template = map[string]*template.Template{}

func init() {
	base := template.Must(template.ParseFS(templateFiles, "templates/base.tmpl"))
	if err := fs.WalkDir(templateFiles, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || path == "templates/base.tmpl" {
			return nil
		}

		if path == "templates/login.tmpl" {
			if util.CheckFileExists("./var/login.tmpl") {
				cclog.Info("overwrite login.tmpl with local file")
				templates[strings.TrimPrefix(path, "templates/")] = template.Must(template.Must(base.Clone()).ParseFiles("./var/login.tmpl"))
				return nil
			}
		}
		if path == "templates/imprint.tmpl" {
			if util.CheckFileExists("./var/imprint.tmpl") {
				cclog.Info("overwrite imprint.tmpl with local file")
				templates[strings.TrimPrefix(path, "templates/")] = template.Must(template.Must(base.Clone()).ParseFiles("./var/imprint.tmpl"))
				return nil
			}
		}
		if path == "templates/privacy.tmpl" {
			if util.CheckFileExists("./var/privacy.tmpl") {
				cclog.Info("overwrite privacy.tmpl with local file")
				templates[strings.TrimPrefix(path, "templates/")] = template.Must(template.Must(base.Clone()).ParseFiles("./var/privacy.tmpl"))
				return nil
			}
		}

		templates[strings.TrimPrefix(path, "templates/")] = template.Must(template.Must(base.Clone()).ParseFS(templateFiles, path))
		return nil
	}); err != nil {
		cclog.Abortf("Web init(): Could not find frontend template files.\nError: %s\n", err.Error())
	}

	_ = base
}

type Build struct {
	Version   string
	Hash      string
	Buildtime string
}

type Page struct {
	Title         string                 // Page title
	MsgType       string                 // For generic use in message boxes
	Message       string                 // For generic use in message boxes
	User          schema.User            // Information about the currently logged in user (Full User Info)
	Roles         map[string]schema.Role // Available roles for frontend render checks
	Build         Build                  // Latest information about the application
	Clusters      []config.ClusterConfig // List of all clusters for use in the Header
	SubClusters   map[string][]string    // Map per cluster of all subClusters for use in the Header
	FilterPresets map[string]any         // For pages with the Filter component, this can be used to set initial filters.
	Infos         map[string]any         // For generic use (e.g. username for /monitoring/user/<id>, job id for /monitoring/job/<id>)
	Config        map[string]any         // UI settings for the currently logged in user (e.g. line width, ...)
	Resampling    *config.ResampleConfig // If not nil, defines resampling trigger and resolutions
	Redirect      string                 // The originally requested URL, for intermediate login handling
}

func RenderTemplate(rw http.ResponseWriter, file string, page *Page) {
	t, ok := templates[file]
	if !ok {
		cclog.Errorf("WEB/WEB > template '%s' not found", file)
	}

	if page.Clusters == nil {
		for _, c := range config.Clusters {
			page.Clusters = append(page.Clusters, config.ClusterConfig{Name: c.Name, FilterRanges: c.FilterRanges, MetricDataRepository: nil})
		}
	}

	if page.SubClusters == nil {
		page.SubClusters = make(map[string][]string)
		for _, cluster := range archive.Clusters {
			for _, sc := range cluster.SubClusters {
				page.SubClusters[cluster.Name] = append(page.SubClusters[cluster.Name], sc.Name)
			}
		}
	}

	cclog.Debugf("Page config : %v\n", page.Config)
	if err := t.Execute(rw, page); err != nil {
		cclog.Errorf("Template error: %s", err.Error())
	}
}
