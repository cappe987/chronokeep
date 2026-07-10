package cmd

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	. "intime/internal"

	"github.com/coder/websocket"
)

// TODO: Add TX packets to websocket display

// go:embed static/materialize.min.js
// go:embed static/materialize.min.css
// go:embed static/bootstrap.min.js
// go:embed static/bootstrap.min.css
// go:embed static/pico.jade.min.css

//go:embed index.html
//go:embed style.css
//go:embed static/htmx.min.js
//go:embed static/ws.min.js
//go:embed static/tailwind.js
var content embed.FS

type WebApp struct {
	App    *App
	TeOpts *TeOpts
}

func wsHandler(wa *WebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		log.Println("WS handler")
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // allow all origins (dev only!)
		})
		if err != nil {
			log.Println("accept error:", err)
			return
		}
		defer conn.Close(websocket.StatusInternalError, "connection closed")

		// Reader goroutine
		// go func() {
		// 	for {
		// 		_, msg, err := conn.Read(ctx)
		// 		if err != nil {
		// 			return
		// 		}
		// 		app.In <- msg
		// 	}
		// }()

		log.Println("WS loop")
		// Writer loop
		for {
			select {
			case <-ctx.Done():
				conn.Close(websocket.StatusNormalClosure, "")
				return

			case msg := <-wa.App.WsOut:
				// TODO: Add command to start recording manually
				if string(msg) == "exit" {
					conn.Close(websocket.StatusNormalClosure, "")
					return
				}
				writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				// log.Printf("WS received from channel: %s\n", msg)
				err := conn.Write(writeCtx, websocket.MessageText, msg)
				cancel()

				if err != nil {
					return
				}
			}
		}
	}
}

func toggle(wa *WebApp) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// if r.URL.Path != "/" {
		// 	http.NotFound(w, r)
		// 	return
		// }
		if r.Method == "POST" {
			// fmt.Fprintf(w, "GET, %q", html.EscapeString(r.URL.Path))
			log.Printf("POST request")

			action := r.PostFormValue("action")
			swtstamp := r.PostFormValue("software")
			peertopeer := r.PostFormValue("peertopeer")
			p1 := r.PostFormValue("port1")
			p2 := r.PostFormValue("port2")
			domain := r.PostFormValue("domain")
			tagged := r.PostFormValue("vlan-tagged")
			vid := r.PostFormValue("vlan")
			prio := r.PostFormValue("prio")
			log.Printf("Action: %s\n", action)
			log.Printf("Swt: %s\n", swtstamp)
			log.Printf("p2p: %s\n", peertopeer)
			log.Printf("domain: %s\n", domain)
			log.Printf("tagged: %s\n", tagged)
			log.Printf("vid: %s\n", vid)
			log.Printf("prio: %s\n", prio)

			if swtstamp == "on" {
				wa.App.Opts.SwTstamp = true
			} else {
				wa.App.Opts.SwTstamp = false
			}
			if peertopeer == "on" {
				wa.TeOpts.Peertopeer = true
			} else {
				wa.TeOpts.Peertopeer = false
			}
			if domain == "" {
				fmt.Printf("Missing domain\n")
			} else {
				i, err := strconv.Atoi(domain)
				if err != nil {
					// ... handle error
					fmt.Printf("Invalid domain\n")
				} else {
					wa.App.Opts.Domain = uint8(i)
				}
			}
			if tagged == "" {
				wa.App.Opts.Vlan = nil
			} else {
				i1, err1 := strconv.Atoi(vid)
				i2, err2 := strconv.Atoi(prio)
				if err1 != nil || err2 != nil {
					fmt.Printf("Missing or invalid VLAN/Prio\n")
				} else {
					v := uint16(i1)
					wa.App.Opts.Vlan = &v
					wa.App.Opts.Prio = uint8(i2)
				}
			}

			wa.TeOpts.Ports[p1] = PortOpts{GM: true}
			wa.TeOpts.Ports[p2] = PortOpts{}
			if p1 == p2 {
				return
			}

			if action == "start" {
				start_app(wa, false)
			} else if action == "start-and-capture" {
				start_app(wa, true)
			} else if action == "record" {
				if wa.App.Running {
					wa.App.In <- []byte(action)
				}
			} else {
				if wa.App.Running {
					wa.App.In <- []byte("exit")
					html := <-wa.App.Out
					fmt.Fprintf(w, string(html))
					wa.App.Running = false
				}
			}

		} else {
			http.Error(w, "Invalid request method.", 405)
		}
	}
}

func get_ports(wa *WebApp) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			log.Printf("POST request get_ports")

			files, err := os.ReadDir("/sys/class/net")
			if err != nil {
				log.Fatal(err)
			}

			str := ""
			names := make([]string, 0)
			for _, file := range files {
				names = append(names, file.Name())
			}
			// TODO: Set default p1 and p2 from config file. Same for other settings.
			sort.Strings(names)
			for _, name := range names {
				str += fmt.Sprintf("<option>%s</option>", name)
			}
			fmt.Fprintf(w, str)
		} else {
			http.Error(w, "Invalid request method.", 405)
		}
	}
}
func get_config(wa *WebApp) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// InitTemplates(app.TeOpts, app.Opts, w)
	}
}

func start_app(wa *WebApp, capture bool) {
	wa.App.Opts.RecordPackets = capture
	go RunTeMode(wa.TeOpts, wa.App)
}

func WebServer() {

	// wsOut := make(chan []byte, 1000)
	teOpts, opts := InitTeOpts()
	app := NewApp(opts, true, false)
	webapp := WebApp{App: app, TeOpts: &teOpts}

	http.Handle("/", http.FileServer(http.FS(content)))
	http.HandleFunc("/ws", wsHandler(&webapp))
	http.HandleFunc("/te-toggle", toggle(&webapp))
	http.HandleFunc("/get-ports", get_ports(&webapp))
	http.HandleFunc("/te-config", get_config(&webapp))
	// http.HandleFunc("/stop", toggle(webapp, false))

	fmt.Printf("Serving on http://localhost:8080\n")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
