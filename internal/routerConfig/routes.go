package routerConfig

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/auth"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/internal/graph"
	"github.com/ClusterCockpit/cc-backend/internal/graph/model"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
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
	{"/monitoring/analysis/{cluster}", "monitoring/analysis.tmpl", "Analaysis - ClusterCockpit", true, setupAnalysisRoute},
	{"/monitoring/status/{cluster}", "monitoring/status.tmpl", "Status of <ID> - ClusterCockpit", false, setupClusterRoute},
}

func setupHomeRoute(i InfoType, r *http.Request) InfoType {
	type cluster struct {
		Name            string
		RunningJobs     int
		TotalJobs       int
		RecentShortJobs int
	}
	jobRepo := repository.GetRepository()

	runningJobs, err := jobRepo.CountGroupedJobs(r.Context(), model.AggregateCluster, []*model.JobFilter{{
		State: []schema.JobState{schema.JobStateRunning},
	}}, nil, nil)
	if err != nil {
		log.Errorf("failed to count jobs: %s", err.Error())
		runningJobs = map[string]int{}
	}
	totalJobs, err := jobRepo.CountGroupedJobs(r.Context(), model.AggregateCluster, nil, nil, nil)
	if err != nil {
		log.Errorf("failed to count jobs: %s", err.Error())
		totalJobs = map[string]int{}
	}
	from := time.Now().Add(-24 * time.Hour)
	recentShortJobs, err := jobRepo.CountGroupedJobs(r.Context(), model.AggregateCluster, []*model.JobFilter{{
		StartTime: &model.TimeRange{From: &from, To: nil},
		Duration:  &model.IntRange{From: 0, To: graph.ShortJobDuration},
	}}, nil, nil)
	if err != nil {
		log.Errorf("failed to count jobs: %s", err.Error())
		recentShortJobs = map[string]int{}
	}

	clusters := make([]cluster, 0)
	for _, c := range config.Clusters {
		clusters = append(clusters, cluster{
			Name:            c.Name,
			RunningJobs:     runningJobs[c.Name],
			TotalJobs:       totalJobs[c.Name],
			RecentShortJobs: recentShortJobs[c.Name],
		})
	}

	i["clusters"] = clusters
	return i
}

func setupJobRoute(i InfoType, r *http.Request) InfoType {
	i["id"] = mux.Vars(r)["id"]
	return i
}

func setupUserRoute(i InfoType, r *http.Request) InfoType {
	jobRepo := repository.GetRepository()
	username := mux.Vars(r)["id"]
	i["id"] = username
	i["username"] = username
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
	var username *string = nil
	jobRepo := repository.GetRepository()
	if user := auth.GetUser(r.Context()); user != nil && !user.HasRole(auth.RoleAdmin) {
		username = &user.Username
	}

	tags, counts, err := jobRepo.CountTags(username)
	tagMap := make(map[string][]map[string]interface{})
	if err != nil {
		log.Errorf("GetTags failed: %s", err.Error())
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
	if query.Get("user") != "" {
		filterPresets["user"] = query.Get("user")
		filterPresets["userMatch"] = "eq"
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

func SetupRoutes(router *mux.Router) {
	for _, route := range routes {
		route := route
		router.HandleFunc(route.Route, func(rw http.ResponseWriter, r *http.Request) {
			conf, err := config.GetUIConfig(r)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			title := route.Title
			infos := route.Setup(map[string]interface{}{}, r)
			if id, ok := infos["id"]; ok {
				title = strings.Replace(route.Title, "<ID>", id.(string), 1)
			}

			username, isAdmin := "", true
			if user := auth.GetUser(r.Context()); user != nil {
				username = user.Username
				isAdmin = user.HasRole(auth.RoleAdmin)
			}

			page := web.Page{
				Title:  title,
				User:   web.User{Username: username, IsAdmin: isAdmin},
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
