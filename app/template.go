//go:build !noweb

package app

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"

	. "intime/app/cmd"
	. "intime/internal"
)

//go:embed config.html
//go:embed timeerror.html
var fs embed.FS

type TeData struct {
	TeOpts   TeOpts
	Opts     CommonOpts
	AllPorts []string
	HasPorts bool
	Port1    string
	Port2    string
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

// Temporary testing with templates. Needs much cleanup
func InitTemplates(to TeOpts, opts CommonOpts, w http.ResponseWriter) {
	tmplFile := "config.html"
	tmpl, err := template.New(tmplFile).ParseFS(fs, tmplFile)
	if err != nil {
		panic(err)
	}
	opts.SwTstamp = true
	// vid := uint16(100)
	// opts.Vlan = &vid
	opts.Prio = 4
	to.Interval = 100
	// TODO: Set default p1 and p2 from config file. Same for other settings.
	td := TeData{
		TeOpts:   to,
		Opts:     opts,
		AllPorts: GetSystemPorts(),
		Port1:    "veth1",
		Port2:    "veth2",
	}
	err = tmpl.Execute(w, td)
	if err != nil {
		panic(err)
	}
}
