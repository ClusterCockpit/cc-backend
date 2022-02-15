package templates

import (
	"html/template"
	"net/http"
	"os"

	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/log"
)

var templatesDir string
var debugMode bool = os.Getenv("DEBUG") == "1"
var templates map[string]*template.Template = map[string]*template.Template{}

type User struct {
	Username string // Username of the currently logged in user
	IsAdmin  bool
}

type Page struct {
	Title         string                 // Page title
	Error         string                 // For generic use (e.g. the exact error message on /login)
	Info          string                 // For generic use (e.g. "Logout successfull" on /login)
	User          User                   // Information about the currently logged in user
	Clusters      []string               // List of all clusters for use in the Header
	FilterPresets map[string]interface{} // For pages with the Filter component, this can be used to set initial filters.
	Infos         map[string]interface{} // For generic use (e.g. username for /monitoring/user/<id>, job id for /monitoring/job/<id>)
	Config        map[string]interface{} // UI settings for the currently logged in user (e.g. line width, ...)
}

func init() {
	bp := "./"
	ebp := os.Getenv("BASEPATH")

	if ebp != "" {
		bp = ebp
	}
	templatesDir = bp + "templates/"
	base := template.Must(template.ParseFiles(templatesDir + "base.tmpl"))
	files := []string{
		"home.tmpl", "404.tmpl", "login.tmpl",
		"imprint.tmpl", "privacy.tmpl",
		"monitoring/jobs.tmpl",
		"monitoring/job.tmpl",
		"monitoring/taglist.tmpl",
		"monitoring/list.tmpl",
		"monitoring/user.tmpl",
		"monitoring/systems.tmpl",
		"monitoring/node.tmpl",
		"monitoring/analysis.tmpl",
	}

	for _, file := range files {
		templates[file] = template.Must(template.Must(base.Clone()).ParseFiles(templatesDir + file))
	}
}

func Render(rw http.ResponseWriter, r *http.Request, file string, page *Page) {
	t, ok := templates[file]
	if !ok {
		panic("templates must be predefinied!")
	}

	if debugMode {
		t = template.Must(template.ParseFiles(templatesDir+"base.tmpl", templatesDir+file))
	}

	if page.Clusters == nil {
		for _, c := range config.Clusters {
			page.Clusters = append(page.Clusters, c.Name)
		}
	}

	if err := t.Execute(rw, page); err != nil {
		log.Errorf("template error: %s", err.Error())
	}
}
