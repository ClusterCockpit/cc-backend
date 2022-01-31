package templates

import (
	"html/template"
	"net/http"
	"os"

	"github.com/ClusterCockpit/cc-backend/log"
)

var templatesDir string
var debugMode bool = os.Getenv("DEBUG") == "1"
var templates map[string]*template.Template = map[string]*template.Template{}

type Page struct {
	Title         string
	Login         *LoginPage
	FilterPresets map[string]interface{}
	Infos         map[string]interface{}
	Config        map[string]interface{}
}

type LoginPage struct {
	Error string
	Info  string
}

func init() {
	templatesDir = "./templates/"
	base := template.Must(template.ParseFiles(templatesDir + "base.html"))
	files := []string{
		"home.html", "404.html", "login.html",
		"monitoring/jobs.html",
		"monitoring/job.html",
		"monitoring/list.html",
		"monitoring/user.html",
		// "monitoring/analysis.html",
		// "monitoring/systems.html",
		// "monitoring/node.html",
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
		t = template.Must(template.ParseFiles(templatesDir+"base.html", templatesDir+file))
	}

	if err := t.Execute(rw, page); err != nil {
		log.Errorf("template error: %s", err.Error())
	}
}
