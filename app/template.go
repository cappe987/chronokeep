//go:build !noweb

package app

import (
	"embed"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed tmpl/*
var tmplFs embed.FS

func GetTemplateFS() embed.FS {
	return tmplFs
}

func GetSystemPorts() []string {
	files, err := os.ReadDir("/sys/class/net")
	if err != nil {
		log.Fatal(err)
	}
	names := make([]string, 0)
	for _, file := range files {
		names = append(names, file.Name())
	}
	sort.Strings(names)
	return names
}

func BuildTemplates() (map[string]*template.Template, error) {
	tc := make(map[string]*template.Template)
	tfs := GetTemplateFS()

	// Read the files and get the pages
	dir := "tmpl/pages"
	pdir := "tmpl/partials"
	pages, err := tfs.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		tmpl := template.New(page.Name())
		tmpl, err := tmpl.ParseFS(tfs,
			pdir+"/chart.html",
			pdir+"/config.html",
			pdir+"/buttons.html",
			dir+"/"+page.Name())
		if err != nil {
			return nil, err
		}
		tc[page.Name()] = tmpl
	}

	partials, err := tfs.ReadDir(pdir)
	if err != nil {
		return nil, err
	}
	for _, partial := range partials {
		pname := strings.TrimSuffix(partial.Name(), filepath.Ext(partial.Name()))
		tmpl := template.New(pname)
		tmpl, err := tmpl.ParseFS(tfs,
			pdir+"/chart.html",
			pdir+"/"+partial.Name())
		if err != nil {
			return nil, err
		}
		tc[pname] = tmpl
	}

	return tc, nil
}
