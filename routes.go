package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/auth"
	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/schema"
	"github.com/ClusterCockpit/cc-backend/templates"
	"github.com/gorilla/mux"
)

type InfoType map[string]interface{}

type Route struct {
	Route    string
	Template string
	Title    string
	Filter   bool
	Setup    func(i InfoType, r *http.Request) InfoType
}

func buildFilterPresets(query url.Values) map[string]interface{} {
	filterPresets := map[string]interface{}{}

	if query.Get("cluster") != "" {
		filterPresets["cluster"] = query.Get("cluster")
	}
	if query.Get("partition") != "" {
		filterPresets["partition"] = query.Get("partition")
	}
	if query.Get("project") != "" {
		filterPresets["project"] = query.Get("project")
		filterPresets["projectMatch"] = "eq"
	}
	if query.Get("user") != "" {
		filterPresets["user"] = query.Get("user")
		filterPresets["userMatch"] = "eq"
	}
	if query.Get("state") != "" && schema.JobState(query.Get("state")).Valid() {
		filterPresets["state"] = query.Get("state")
	}
	if rawtags, ok := query["tag"]; ok {
		tags := make([]int, len(rawtags))
		for i, tid := range rawtags {
			var err error
			tags[i], err = strconv.Atoi(tid)
			if err != nil {
				tags[i] = -1
			}
		}
		filterPresets["tags"] = tags
	}
	if query.Get("numNodes") != "" {
		parts := strings.Split(query.Get("numNodes"), "-")
		if len(parts) == 2 {
			a, e1 := strconv.Atoi(parts[0])
			b, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil {
				filterPresets["numNodes"] = map[string]int{"from": a, "to": b}
			}
		}
	}
	if query.Get("jobId") != "" {
		filterPresets["jobId"] = query.Get("jobId")
	}
	if query.Get("arrayJobId") != "" {
		if num, err := strconv.Atoi(query.Get("arrayJobId")); err == nil {
			filterPresets["arrayJobId"] = num
		}
	}
	if query.Get("startTime") != "" {
		parts := strings.Split(query.Get("startTime"), "-")
		if len(parts) == 2 {
			a, e1 := strconv.ParseInt(parts[0], 10, 64)
			b, e2 := strconv.ParseInt(parts[1], 10, 64)
			if e1 == nil && e2 == nil {
				filterPresets["startTime"] = map[string]string{
					"from": time.Unix(a, 0).Format(time.RFC3339),
					"to":   time.Unix(b, 0).Format(time.RFC3339),
				}
			}
		}
	}

	return filterPresets
}

func setupRoutes(router *mux.Router, routes []Route) {
	for _, route := range routes {
		route := route
		router.HandleFunc(route.Route, func(rw http.ResponseWriter, r *http.Request) {
			conf, err := config.GetUIConfig(r)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			infos := route.Setup(map[string]interface{}{}, r)
			if id, ok := infos["id"]; ok {
				route.Title = strings.Replace(route.Title, "<ID>", id.(string), 1)
			}

			username, isAdmin := "", true
			if user := auth.GetUser(r.Context()); user != nil {
				username = user.Username
				isAdmin = user.HasRole(auth.RoleAdmin)
			}

			page := templates.Page{
				Title:  route.Title,
				User:   templates.User{Username: username, IsAdmin: isAdmin},
				Config: conf,
				Infos:  infos,
			}

			if route.Filter {
				page.FilterPresets = buildFilterPresets(r.URL.Query())
			}

			templates.Render(rw, r, route.Template, &page)
		})
	}
}
