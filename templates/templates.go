package templates

import (
	"html/template"
	"log"
	"net/http"
)

var templates *template.Template

type Page struct {
	Title string
	Login *LoginPage
}

type LoginPage struct {
	Error string
	Info  string
}

func init() {
	templates = template.Must(template.ParseGlob("./templates/*.html"))
}

func Render(rw http.ResponseWriter, r *http.Request, name string, page *Page) {
	if err := templates.ExecuteTemplate(rw, name, page); err != nil {
		log.Printf("template error: %s\n", err.Error())
	}
}
