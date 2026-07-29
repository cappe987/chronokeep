//go:build !noweb

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package app

import (
	"embed"
	"fmt"
	"html/template"
	"slices"

	. "ckeep/internal"
)

//go:embed tmpl/*
var tmplFs embed.FS

func GetTemplateFS() embed.FS {
	return tmplFs
}

type Tmpl struct {
	name string
	deps []Tmpl
}

func (t *Tmpl) getDeps() []string {
	var arr []string
	for _, t := range t.deps {
		arr = append(arr, t.getDeps()...)
		arr = append(arr, t.name)
	}
	return arr
}

func (t *Tmpl) getUniqueDeps() []string {
	deplist := t.getDeps()
	var unique []string
	for _, dep := range deplist {
		if !slices.Contains(unique, dep) {
			unique = append(unique, dep)
		}
	}
	return unique
}

// Gathers all the files in correct dependency order and filters out duplicates
func (t *Tmpl) getFiles() []string {
	deplist := t.getUniqueDeps()
	var filenames []string
	for _, dep := range deplist {
		filenames = append(filenames, fmt.Sprintf("tmpl/%s.html", dep))
	}
	LogDebug("Init template '%s' with dependencies %v", t.name, filenames)
	filenames = append(filenames, fmt.Sprintf("tmpl/%s.html", t.name))
	return filenames
}

func BuildTemplates() (map[string]*template.Template, error) {
	tc := make(map[string]*template.Template)
	tfs := GetTemplateFS()

	// TODO: Can we just grep through files for 'template ".*"' to figure this out
	// Is there really no better way to handle dependencies?
	chart := Tmpl{name: "chart"}
	config := Tmpl{name: "config"}
	navbar := Tmpl{name: "navbar"}
	buttons := Tmpl{name: "buttons", deps: []Tmpl{navbar, chart}}
	te := Tmpl{name: "te", deps: []Tmpl{buttons, config, navbar, chart}}
	help := Tmpl{name: "help", deps: []Tmpl{navbar}}
	examples := Tmpl{name: "examples", deps: []Tmpl{navbar}}
	packet := Tmpl{name: "packet", deps: []Tmpl{navbar}}
	index := Tmpl{name: "index", deps: []Tmpl{navbar, te, help, examples, packet}}

	templates := []Tmpl{
		chart,
		config,
		navbar,
		buttons,
		te,
		help,
		examples,
		packet,
		index,
	}

	for _, t := range templates {
		deplist := t.getFiles()
		tmpl := template.New(t.name)
		tmpl, err := tmpl.ParseFS(tfs, deplist...)
		if err != nil {
			return nil, err
		}
		tc[t.name] = tmpl
	}

	return tc, nil
}
