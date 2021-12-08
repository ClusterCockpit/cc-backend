package templates

import (
	"html/template"
	"log"
	"net/http"
)

var templates map[string]*template.Template

type Page struct {
	Title  string
	Login  *LoginPage
	Infos  map[string]interface{}
	Config map[string]interface{}
}

type LoginPage struct {
	Error string
	Info  string
}

func init() {
	base := template.Must(template.ParseFiles("./templates/base.html"))
	templates = map[string]*template.Template{
		"home":              template.Must(template.Must(base.Clone()).ParseFiles("./templates/home.html")),
		"404":               template.Must(template.Must(base.Clone()).ParseFiles("./templates/404.html")),
		"login":             template.Must(template.Must(base.Clone()).ParseFiles("./templates/login.html")),
		"monitoring/jobs/":  template.Must(template.Must(base.Clone()).ParseFiles("./templates/monitoring/jobs.html")),
		"monitoring/job/":   template.Must(template.Must(base.Clone()).ParseFiles("./templates/monitoring/job.html")),
		"monitoring/users/": template.Must(template.Must(base.Clone()).ParseFiles("./templates/monitoring/users.html")),
		"monitoring/user/":  template.Must(template.Must(base.Clone()).ParseFiles("./templates/monitoring/user.html")),
	}
}

func Render(rw http.ResponseWriter, r *http.Request, name string, page *Page) {
	if err := templates[name].Execute(rw, page); err != nil {
		log.Printf("template error: %s\n", err.Error())
	}
}
