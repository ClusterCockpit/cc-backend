// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package routerConfig

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/internal/util"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/web"
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

var routes []Route = []Route{
	{"/", "home.tmpl", "ClusterCockpit", false, setupHomeRoute},
	{"/config", "config.tmpl", "Settings", false, func(i InfoType, r *http.Request) InfoType { return i }},
	{"/monitoring/jobs/", "monitoring/jobs.tmpl", "Jobs - ClusterCockpit", true, func(i InfoType, r *http.Request) InfoType { return i }},
	{"/monitoring/job/{id:[0-9]+}", "monitoring/job.tmpl", "Job <ID> - ClusterCockpit", false, setupJobRoute},
	{"/monitoring/users/", "monitoring/list.tmpl", "Users - ClusterCockpit", true, func(i InfoType, r *http.Request) InfoType { i["listType"] = "USER"; return i }},
	{"/monitoring/projects/", "monitoring/list.tmpl", "Projects - ClusterCockpit", true, func(i InfoType, r *http.Request) InfoType { i["listType"] = "PROJECT"; return i }},
	{"/monitoring/tags/", "monitoring/taglist.tmpl", "Tags - ClusterCockpit", false, setupTaglistRoute},
	{"/monitoring/user/{id}", "monitoring/user.tmpl", "User <ID> - ClusterCockpit", true, setupUserRoute},
	{"/monitoring/systems/{cluster}", "monitoring/systems.tmpl", "Cluster <ID> - ClusterCockpit", false, setupClusterRoute},
	{"/monitoring/node/{cluster}/{hostname}", "monitoring/node.tmpl", "Node <ID> - ClusterCockpit", false, setupNodeRoute},
	{"/monitoring/analysis/{cluster}", "monitoring/analysis.tmpl", "Analysis - ClusterCockpit", true, setupAnalysisRoute},
	{"/monitoring/status/{cluster}", "monitoring/status.tmpl", "Status of <ID> - ClusterCockpit", false, setupClusterRoute},
}

func setupHomeRoute(i InfoType, r *http.Request) InfoType {
	jobRepo := repository.GetJobRepository()
	groupBy := model.AggregateCluster

	stats, err := jobRepo.JobCountGrouped(r.Context(), nil, &groupBy)
	if err != nil {
		log.Warnf("failed to count jobs: %s", err.Error())
	}

	stats, err = jobRepo.AddJobCountGrouped(r.Context(), nil, &groupBy, stats, "running")
	if err != nil {
		log.Warnf("failed to count running jobs: %s", err.Error())
	}

	i["clusters"] = stats

	if util.CheckFileExists("./var/notice.txt") {
		msg, err := os.ReadFile("./var/notice.txt")
		if err != nil {
			log.Warnf("failed to read notice.txt file: %s", err.Error())
		} else {
			i["message"] = string(msg)
		}
	}

	return i
}

func setupJobRoute(i InfoType, r *http.Request) InfoType {
	i["id"] = mux.Vars(r)["id"]
	return i
}

func setupUserRoute(i InfoType, r *http.Request) InfoType {
	jobRepo := repository.GetJobRepository()
	username := mux.Vars(r)["id"]
	i["id"] = username
	i["username"] = username
	// TODO: If forbidden (== err exists), redirect to error page
	if user, _ := auth.FetchUser(r.Context(), jobRepo.DB, username); user != nil {
		i["name"] = user.Name
		i["email"] = user.Email
	}
	return i
}

func setupClusterRoute(i InfoType, r *http.Request) InfoType {
	vars := mux.Vars(r)
	i["id"] = vars["cluster"]
	i["cluster"] = vars["cluster"]
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from != "" || to != "" {
		i["from"] = from
		i["to"] = to
	}
	return i
}

func setupNodeRoute(i InfoType, r *http.Request) InfoType {
	vars := mux.Vars(r)
	i["cluster"] = vars["cluster"]
	i["hostname"] = vars["hostname"]
	i["id"] = fmt.Sprintf("%s (%s)", vars["cluster"], vars["hostname"])
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from != "" || to != "" {
		i["from"] = from
		i["to"] = to
	}
	return i
}

func setupAnalysisRoute(i InfoType, r *http.Request) InfoType {
	i["cluster"] = mux.Vars(r)["cluster"]
	return i
}

func setupTaglistRoute(i InfoType, r *http.Request) InfoType {
	jobRepo := repository.GetJobRepository()
	user := auth.GetUser(r.Context())

	tags, counts, err := jobRepo.CountTags(user)
	tagMap := make(map[string][]map[string]interface{})
	if err != nil {
		log.Warnf("GetTags failed: %s", err.Error())
		i["tagmap"] = tagMap
		return i
	}

	for _, tag := range tags {
		tagItem := map[string]interface{}{
			"id":    tag.ID,
			"name":  tag.Name,
			"count": counts[tag.Name],
		}
		tagMap[tag.Type] = append(tagMap[tag.Type], tagItem)
	}
	i["tagmap"] = tagMap
	return i
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
	if query.Get("jobName") != "" {
		filterPresets["jobName"] = query.Get("jobName")
	}
	if len(query["user"]) != 0 {
		if len(query["user"]) == 1 {
			filterPresets["user"] = query.Get("user")
			filterPresets["userMatch"] = "contains"
		} else {
			filterPresets["user"] = query["user"]
			filterPresets["userMatch"] = "in"
		}
	}
	if len(query["state"]) != 0 {
		filterPresets["state"] = query["state"]
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
	if query.Get("duration") != "" {
		parts := strings.Split(query.Get("duration"), "-")
		if len(parts) == 2 {
			a, e1 := strconv.Atoi(parts[0])
			b, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil {
				filterPresets["duration"] = map[string]int{"from": a, "to": b}
			}
		}
	}
	if query.Get("node") != "" {
		filterPresets["node"] = query.Get("node")
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
	if query.Get("numAccelerators") != "" {
		parts := strings.Split(query.Get("numAccelerators"), "-")
		if len(parts) == 2 {
			a, e1 := strconv.Atoi(parts[0])
			b, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil {
				filterPresets["numAccelerators"] = map[string]int{"from": a, "to": b}
			}
		}
	}
	if query.Get("jobId") != "" {
		if len(query["jobId"]) == 1 {
			filterPresets["jobId"] = query.Get("jobId")
			filterPresets["jobIdMatch"] = "eq"
		} else {
			filterPresets["jobId"] = query["jobId"]
			filterPresets["jobIdMatch"] = "in"
		}
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

func SetupRoutes(router *mux.Router, buildInfo web.Build) {
	userCfgRepo := repository.GetUserCfgRepo()
	for _, route := range routes {
		route := route
		router.HandleFunc(route.Route, func(rw http.ResponseWriter, r *http.Request) {
			conf, err := userCfgRepo.GetUIConfig(auth.GetUser(r.Context()))
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			title := route.Title
			infos := route.Setup(map[string]interface{}{}, r)
			if id, ok := infos["id"]; ok {
				title = strings.Replace(route.Title, "<ID>", id.(string), 1)
			}

			// Get User -> What if NIL?
			user := auth.GetUser(r.Context())
			// Get Roles
			availableRoles, _ := auth.GetValidRolesMap(user)

			page := web.Page{
				Title:  title,
				User:   *user,
				Roles:  availableRoles,
				Build:  buildInfo,
				Config: conf,
				Infos:  infos,
			}

			if route.Filter {
				page.FilterPresets = buildFilterPresets(r.URL.Query())
			}

			web.RenderTemplate(rw, r, route.Template, &page)
		})
	}
}

func HandleSearchBar(rw http.ResponseWriter, r *http.Request, buildInfo web.Build) {
	user := auth.GetUser(r.Context())
	availableRoles, _ := auth.GetValidRolesMap(user)

	if search := r.URL.Query().Get("searchId"); search != "" {
		repo := repository.GetJobRepository()
		splitSearch := strings.Split(search, ":")

		if len(splitSearch) == 2 {
			switch strings.Trim(splitSearch[0], " ") {
			case "jobId":
				http.Redirect(rw, r, "/monitoring/jobs/?jobId="+url.QueryEscape(strings.Trim(splitSearch[1], " ")), http.StatusFound) // All Users: Redirect to Tablequery
			case "jobName":
				http.Redirect(rw, r, "/monitoring/jobs/?jobName="+url.QueryEscape(strings.Trim(splitSearch[1], " ")), http.StatusFound) // All Users: Redirect to Tablequery
			case "projectId":
				http.Redirect(rw, r, "/monitoring/jobs/?projectMatch=eq&project="+url.QueryEscape(strings.Trim(splitSearch[1], " ")), http.StatusFound) // All Users: Redirect to Tablequery
			case "arrayJobId":
				http.Redirect(rw, r, "/monitoring/jobs/?arrayJobId="+url.QueryEscape(strings.Trim(splitSearch[1], " ")), http.StatusFound) // All Users: Redirect to Tablequery
			case "username":
				if user.HasAnyRole([]auth.Role{auth.RoleAdmin, auth.RoleSupport, auth.RoleManager}) {
					http.Redirect(rw, r, "/monitoring/users/?user="+url.QueryEscape(strings.Trim(splitSearch[1], " ")), http.StatusFound)
				} else {
					web.RenderTemplate(rw, r, "message.tmpl", &web.Page{Title: "Error", MsgType: "alert-danger", Message: "Missing Access Rights", User: *user, Roles: availableRoles, Build: buildInfo})
				}
			case "name":
				usernames, _ := repo.FindColumnValues(user, strings.Trim(splitSearch[1], " "), "user", "username", "name")
				if len(usernames) != 0 {
					joinedNames := strings.Join(usernames, "&user=")
					http.Redirect(rw, r, "/monitoring/users/?user="+joinedNames, http.StatusFound)
				} else {
					if user.HasAnyRole([]auth.Role{auth.RoleAdmin, auth.RoleSupport, auth.RoleManager}) {
						http.Redirect(rw, r, "/monitoring/users/?user=NoUserNameFound", http.StatusPermanentRedirect)
					} else {
						web.RenderTemplate(rw, r, "message.tmpl", &web.Page{Title: "Error", MsgType: "alert-danger", Message: "Missing Access Rights", User: *user, Roles: availableRoles, Build: buildInfo})
					}
				}
			default:
				web.RenderTemplate(rw, r, "message.tmpl", &web.Page{Title: "Warning", MsgType: "alert-warning", Message: fmt.Sprintf("Unknown search type: %s", strings.Trim(splitSearch[0], " ")), User: *user, Roles: availableRoles, Build: buildInfo})
			}
		} else if len(splitSearch) == 1 {

			jobid, username, project, jobname := repo.FindUserOrProjectOrJobname(user, strings.Trim(search, " "))

			if jobid != "" {
				http.Redirect(rw, r, "/monitoring/jobs/?jobId="+url.QueryEscape(jobid), http.StatusFound) // JobId (Match)
			} else if username != "" {
				http.Redirect(rw, r, "/monitoring/user/"+username, http.StatusFound) // User: Redirect to user page of first match
			} else if project != "" {
				http.Redirect(rw, r, "/monitoring/jobs/?projectMatch=eq&project="+url.QueryEscape(project), http.StatusFound) // projectId (equal)
			} else if jobname != "" {
				http.Redirect(rw, r, "/monitoring/jobs/?jobName="+url.QueryEscape(jobname), http.StatusFound) // JobName (contains)
			} else {
				web.RenderTemplate(rw, r, "message.tmpl", &web.Page{Title: "Info", MsgType: "alert-info", Message: "Search without result", User: *user, Roles: availableRoles, Build: buildInfo})
			}

		} else {
			web.RenderTemplate(rw, r, "message.tmpl", &web.Page{Title: "Error", MsgType: "alert-danger", Message: "Searchbar query parameters malformed", User: *user, Roles: availableRoles, Build: buildInfo})
		}
	} else {
		web.RenderTemplate(rw, r, "message.tmpl", &web.Page{Title: "Warning", MsgType: "alert-warning", Message: "Empty search", User: *user, Roles: availableRoles, Build: buildInfo})
	}
}
