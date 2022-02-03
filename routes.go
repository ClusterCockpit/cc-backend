package main

import (
	"net/http"
	"strings"

	"github.com/ClusterCockpit/cc-backend/auth"
	"github.com/ClusterCockpit/cc-backend/config"
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

func setupRoutes(router *mux.Router, routes []Route) {
	for _, route := range routes {
		_route := route
		router.HandleFunc(_route.Route, func(rw http.ResponseWriter, r *http.Request) {
			conf, err := config.GetUIConfig(r)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			infos := map[string]interface{}{
				"admin": true,
			}

			if user := auth.GetUser(r.Context()); user != nil {
				infos["username"] = user.Username
				infos["admin"] = user.HasRole(auth.RoleAdmin)
			} else {
				infos["username"] = false
				infos["admin"] = false
			}

			infos = _route.Setup(infos, r)
			if id, ok := infos["id"]; ok {
				_route.Title = strings.Replace(_route.Title, "<ID>", id.(string), 1)
			}

			templates.Render(rw, r, _route.Template, &templates.Page{
				Title:         _route.Title,
				Config:        conf,
				Infos:         infos,
				FilterPresets: buildFilterPresets(r.URL.Query()),
			})
		})
	}
}
