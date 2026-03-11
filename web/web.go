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

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/ClusterCockpit/cc-lib/v2/util"
)

type WebConfig struct {
	JobList           JobListConfig     `json:"job-list"`
	NodeList          NodeListConfig    `json:"node-list"`
	JobView           JobViewConfig     `json:"job-view"`
	MetricConfig      MetricConfig      `json:"metric-config"`
	PlotConfiguration PlotConfiguration `json:"plot-configuration"`
}

type JobListConfig struct {
	UsePaging     bool `json:"use-paging"`
	ShowFootprint bool `json:"show-footprint"`
}

type NodeListConfig struct {
	UsePaging bool `json:"use-paging"`
}

type JobViewConfig struct {
	ShowPolarPlot bool `json:"show-polar-plot"`
	ShowFootprint bool `json:"show-footprint"`
	ShowRoofline  bool `json:"show-roofline"`
	ShowStatTable bool `json:"show-stat-table"`
}

type MetricConfig struct {
	JobListMetrics      []string        `json:"job-list-metrics"`
	JobViewPlotMetrics  []string        `json:"job-view-plot-metrics"`
	JobViewTableMetrics []string        `json:"job-view-table-metrics"`
	Clusters            []ClusterConfig `json:"clusters"`
}

type ClusterConfig struct {
	Name                string             `json:"name"`
	JobListMetrics      []string           `json:"job-list-metrics"`
	JobViewPlotMetrics  []string           `json:"job-view-plot-metrics"`
	JobViewTableMetrics []string           `json:"job-view-table-metrics"`
	SubClusters         []SubClusterConfig `json:"sub-clusters"`
}

type SubClusterConfig struct {
	Name                string   `json:"name"`
	JobListMetrics      []string `json:"job-list-metrics"`
	JobViewPlotMetrics  []string `json:"job-view-plot-metrics"`
	JobViewTableMetrics []string `json:"job-view-table-metrics"`
}

type PlotConfiguration struct {
	ColorBackground bool     `json:"color-background"`
	PlotsPerRow     int      `json:"plots-per-row"`
	LineWidth       int      `json:"line-width"`
	ColorScheme     []string `json:"color-scheme"`
}

var UIDefaults = WebConfig{
	JobList: JobListConfig{
		UsePaging:     false,
		ShowFootprint: false,
	},
	NodeList: NodeListConfig{
		UsePaging: false,
	},
	JobView: JobViewConfig{
		ShowPolarPlot: true,
		ShowFootprint: false,
		ShowRoofline:  true,
		ShowStatTable: true,
	},
	MetricConfig: MetricConfig{
		JobListMetrics:      []string{"cpu_load", "flops_any", "mem_bw", "mem_used"},
		JobViewPlotMetrics:  []string{"cpu_load", "flops_any", "mem_bw", "mem_used"},
		JobViewTableMetrics: []string{"flops_any", "mem_bw", "mem_used"},
	},
	PlotConfiguration: PlotConfiguration{
		ColorBackground: true,
		PlotsPerRow:     3,
		LineWidth:       3,
		ColorScheme:     []string{"#00bfff", "#0000ff", "#ff00ff", "#ff0000", "#ff8000", "#ffff00", "#80ff00"},
	},
}

var UIDefaultsMap map[string]any

//
// 	map[string]any{
// 	"analysis_view_histogramMetrics":         []string{"flops_any", "mem_bw", "mem_used"},
// 	"analysis_view_scatterPlotMetrics":       [][]string{{"flops_any", "mem_bw"}, {"flops_any", "cpu_load"}, {"cpu_load", "mem_bw"}},
// 	"job_view_nodestats_selectedMetrics":     []string{"flops_any", "mem_bw", "mem_used"},
// 	"plot_list_jobsPerPage":                  50,
// 	"analysis_view_selectedTopEntity":        "user",
// 	"analysis_view_selectedTopCategory":      "totalWalltime",
// 	"status_view_selectedTopUserCategory":    "totalJobs",
// 	"status_view_selectedTopProjectCategory": "totalJobs",
// }

func Init(rawConfig json.RawMessage) error {
	var err error

	if rawConfig != nil {
		config.Validate(configSchema, rawConfig)
		if err = json.Unmarshal(rawConfig, &UIDefaults); err != nil {
			cclog.Warn("Error while unmarshaling raw config json")
			return err
		}
	}

	UIDefaultsMap = make(map[string]any)

	UIDefaultsMap["jobList_usePaging"] = UIDefaults.JobList.UsePaging
	UIDefaultsMap["jobList_showFootprint"] = UIDefaults.JobList.ShowFootprint
	UIDefaultsMap["nodeList_usePaging"] = UIDefaults.NodeList.UsePaging
	UIDefaultsMap["jobView_showPolarPlot"] = UIDefaults.JobView.ShowPolarPlot
	UIDefaultsMap["jobView_showFootprint"] = UIDefaults.JobView.ShowFootprint
	UIDefaultsMap["jobView_showRoofline"] = UIDefaults.JobView.ShowRoofline
	UIDefaultsMap["jobView_showStatTable"] = UIDefaults.JobView.ShowStatTable

	UIDefaultsMap["metricConfig_jobListMetrics"] = UIDefaults.MetricConfig.JobListMetrics
	UIDefaultsMap["metricConfig_jobViewPlotMetrics"] = UIDefaults.MetricConfig.JobViewPlotMetrics
	UIDefaultsMap["metricConfig_jobViewTableMetrics"] = UIDefaults.MetricConfig.JobViewTableMetrics

	UIDefaultsMap["plotConfiguration_colorBackground"] = UIDefaults.PlotConfiguration.ColorBackground
	UIDefaultsMap["plotConfiguration_plotsPerRow"] = UIDefaults.PlotConfiguration.PlotsPerRow
	UIDefaultsMap["plotConfiguration_lineWidth"] = UIDefaults.PlotConfiguration.LineWidth
	UIDefaultsMap["plotConfiguration_colorScheme"] = UIDefaults.PlotConfiguration.ColorScheme

	for _, c := range UIDefaults.MetricConfig.Clusters {
		if c.JobListMetrics != nil {
			UIDefaultsMap["metricConfig_jobListMetrics:"+c.Name] = c.JobListMetrics
		}
		if c.JobViewPlotMetrics != nil {
			UIDefaultsMap["metricConfig_jobViewPlotMetrics:"+c.Name] = c.JobViewPlotMetrics
		}
		if c.JobViewTableMetrics != nil {
			UIDefaultsMap["metricConfig_jobViewTableMetrics:"+c.Name] = c.JobViewTableMetrics
		}

		for _, sc := range c.SubClusters {
			suffix := strings.Join([]string{c.Name, sc.Name}, ":")
			if sc.JobListMetrics != nil {
				UIDefaultsMap["metricConfig_jobListMetrics:"+suffix] = sc.JobListMetrics
			}
			if sc.JobViewPlotMetrics != nil {
				UIDefaultsMap["metricConfig_jobViewPlotMetrics:"+suffix] = sc.JobViewPlotMetrics
			}
			if sc.JobViewTableMetrics != nil {
				UIDefaultsMap["metricConfig_jobViewTableMetrics:"+suffix] = sc.JobViewTableMetrics
			}
		}
	}

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

// StaticFileExists checks whether a static file exists in the embedded frontend FS.
func StaticFileExists(path string) bool {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return false
	}
	_, err := fs.Stat(frontendFiles, "frontend/public/"+path)
	return err == nil
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

		if path == "templates/404.tmpl" {
			templates[strings.TrimPrefix(path, "templates/")] = template.Must(template.ParseFS(templateFiles, path))
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
	Clusters      []string               // List of all cluster names
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
		page.Clusters = make([]string, 0)
	}

	if page.SubClusters == nil {
		page.SubClusters = make(map[string][]string)
		for _, cluster := range archive.Clusters {
			page.Clusters = append(page.Clusters, cluster.Name)

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
