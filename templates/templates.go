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
	Error         string
	Info          string
	FilterPresets map[string]interface{}
	Infos         map[string]interface{}
	Config        map[string]interface{}
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

	if err := t.Execute(rw, page); err != nil {
		log.Errorf("template error: %s", err.Error())
	}
}
