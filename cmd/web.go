package cmd

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
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

type App struct {
	In      chan []byte // To App
	Out     chan []byte // From App
	WsOut   chan []byte // Websocket data from App to client
	Running bool
}

func NewApp(init_channels bool) *App {
	if init_channels {
		return &App{
			In:    make(chan []byte, 100),
			Out:   make(chan []byte, 100),
			WsOut: make(chan []byte, 100),
		}
	} else {
		return &App{}
	}
}

func wsHandler(app *App) http.HandlerFunc {
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

			case msg := <-app.WsOut:
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
func toggle(app *App) func(w http.ResponseWriter, r *http.Request) {
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
			log.Printf("Action: %s\n", action)
			log.Printf("Swt: %s\n", swtstamp)
			log.Printf("p2p: %s\n", peertopeer)

			sw := false
			if swtstamp == "on" {
				sw = true
			}
			p2p := false
			if peertopeer == "on" {
				p2p = true
			}
			// p1 := ""
			// p2 := ""
			// log.Printf("Body %v\n", r)
			// for _, value := range r.Form {
			// 	log.Printf("Value %v\n", value)
			// 	// if key == "port1" {
			// 	// p1 = value
			// 	// } else if key == "port2" {
			// 	// p2 = value
			// 	// }
			// }
			if action == "start" {
				start_app(app, p1, p2, sw, p2p, false)
			} else if action == "start-and-capture" {
				start_app(app, p1, p2, sw, p2p, true)
			} else if action == "record" {
				app.In <- []byte(action)
			} else {
				app.In <- []byte("exit")
				html := <-app.Out
				fmt.Fprintf(w, string(html))
			}

		} else {
			http.Error(w, "Invalid request method.", 405)
		}
	}
}

func get_ports(app *App) func(w http.ResponseWriter, r *http.Request) {
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
func get_config(app *App) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		teOpts, opts := InitWebTeOpts()
		InitTemplates(teOpts, opts, w)
	}
}

func InitWebTeOpts() (TeOpts, CommonOpts) {
	var teOpts = TeOpts{}
	var opts = CommonOpts{Mode: "te"}
	opts.InitDefaults()
	// opts.RecordPackets = false
	// teOpts.Count = 1
	teOpts.Interval = uint32(1000)
	teOpts.DelayRecord = uint32(0)

	teOpts.Ports = make(map[string]PortOpts)
	// teOpts.Ports["eth21"] = PortOpts{GM: true}
	// teOpts.Ports["eth22"] = PortOpts{}
	opts.Validate()
	return teOpts, opts
}

func start_app(app *App, p1, p2 string, swtstamp, peertopeer, capture bool) {
	teOpts, opts := InitWebTeOpts()
	teOpts.Ports[p1] = PortOpts{GM: true}
	teOpts.Ports[p2] = PortOpts{}
	opts.SwTstamp = swtstamp
	teOpts.Peertopeer = peertopeer
	opts.RecordPackets = capture
	go RunTeMode(opts, teOpts, app)
}

func WebServer() {

	// wsOut := make(chan []byte, 1000)
	app := NewApp(true)

	http.Handle("/", http.FileServer(http.FS(content)))
	http.HandleFunc("/ws", wsHandler(app))
	http.HandleFunc("/te-toggle", toggle(app))
	http.HandleFunc("/get-ports", get_ports(app))
	http.HandleFunc("/te-config", get_config(app))
	// http.HandleFunc("/stop", toggle(app, false))

	fmt.Printf("Serving on http://localhost:8080\n")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
