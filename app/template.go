//go:build !noweb

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package app

import (
	"embed"
	"html/template"
	"path/filepath"
	"strings"
)

//go:embed tmpl/*
var tmplFs embed.FS

func GetTemplateFS() embed.FS {
	return tmplFs
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

	// Dependency order
	partialsS := []string{
		pdir + "/chart.html",
		pdir + "/config.html",
		pdir + "/navbar.html",
		pdir + "/buttons.html",
		pdir + "/te.html",
		pdir + "/help.html",
		pdir + "/examples.html",
		pdir + "/packet.html",
	}

	for _, page := range pages {
		tmpl := template.New(page.Name())
		partialsAll := append(partialsS, dir+"/"+page.Name())
		tmpl, err := tmpl.ParseFS(tfs,
			partialsAll...)
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
		partialsAll := append(partialsS, pdir+"/"+partial.Name())
		tmpl, err := tmpl.ParseFS(tfs, partialsAll...)
		// pdir+"/chart.html",
		// pdir+"/"+partial.Name())
		if err != nil {
			return nil, err
		}
		tc[pname] = tmpl
	}

	return tc, nil
}
