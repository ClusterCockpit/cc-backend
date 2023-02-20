// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
)

/// Go's embed is only allowed to embed files in a subdirectory of the embedding package ([see here](https://github.com/golang/go/issues/46056)).

//go:embed frontend/public/*
var frontendFiles embed.FS

func ServeFiles() http.Handler {
	publicFiles, err := fs.Sub(frontendFiles, "frontend/public")
	if err != nil {
		panic(err)
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

		templates[strings.TrimPrefix(path, "templates/")] = template.Must(template.Must(base.Clone()).ParseFS(templateFiles, path))
		return nil
	}); err != nil {
		panic(err)
	}

	_ = base
}

type User struct {
	Username  string // Username of the currently logged in user
	AuthLevel int    // Level of authorization
}

type Build struct {
	Version   string
	Hash      string
	Buildtime string
}

type Page struct {
	Title         string                 // Page title
	Error         string                 // For generic use (e.g. the exact error message on /login)
	Info          string                 // For generic use (e.g. "Logout successfull" on /login)
	User          User                   // Information about the currently logged in user
	Build         Build                  // Latest information about the application
	Clusters      []schema.ClusterConfig // List of all clusters for use in the Header
	FilterPresets map[string]interface{} // For pages with the Filter component, this can be used to set initial filters.
	Infos         map[string]interface{} // For generic use (e.g. username for /monitoring/user/<id>, job id for /monitoring/job/<id>)
	Config        map[string]interface{} // UI settings for the currently logged in user (e.g. line width, ...)
}

func RenderTemplate(rw http.ResponseWriter, r *http.Request, file string, page *Page) {
	t, ok := templates[file]
	if !ok {
		panic("template not found")
	}

	if page.Clusters == nil {
		for _, c := range config.Keys.Clusters {
			page.Clusters = append(page.Clusters, schema.ClusterConfig{Name: c.Name, FilterRanges: c.FilterRanges, MetricDataRepository: nil})
		}
	}

	log.Infof("%v\n", page.Config)
	if err := t.Execute(rw, page); err != nil {
		log.Errorf("template error: %s", err.Error())
	}
}
