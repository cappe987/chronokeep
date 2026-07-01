package cmd

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"time"

	. "intime/internal"

	"github.com/coder/websocket"
)

// go:embed static/materialize.min.js
// go:embed static/materialize.min.css
// go:embed static/bootstrap.min.js
// go:embed static/bootstrap.min.css

//go:embed index.html
//go:embed style.css
//go:embed static/htmx.min.js
//go:embed static/ws.min.js
//go:embed static/pico.jade.min.css
var content embed.FS

type App struct {
	In  chan []byte // messages from websocket
	Out chan []byte // messages to websocket
}

func NewApp() *App {
	return &App{
		In:  make(chan []byte, 100),
		Out: make(chan []byte, 100),
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

			case msg := <-app.Out:
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
func toggle(app *App, enable bool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// if r.URL.Path != "/" {
		// 	http.NotFound(w, r)
		// 	return
		// }
		log.Printf("Toggle request %v. Method %s", enable, r.Method)
		if r.Method == "POST" {
			// fmt.Fprintf(w, "GET, %q", html.EscapeString(r.URL.Path))
			log.Printf("POST request %v", enable)

			p1 := r.PostFormValue("port1")
			p2 := r.PostFormValue("port2")

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
			if enable {
				start_app(app, p1, p2)
			} else {
				app.In <- []byte("exit")
			}

		} else {
			http.Error(w, "Invalid request method.", 405)
		}
	}
}

func start_app(app *App, p1, p2 string) {
	var teOpts = TeOpts{}
	var opts = CommonOpts{Mode: "te"}
	// teOpts.Count = 1
	teOpts.Interval = uint32(1000)
	teOpts.DelayRecord = uint32(0)
	opts.Iface = "dummy"
	opts.Ip = "dummy"

	teOpts.Ports = make(map[string]PortOpts)
	teOpts.Ports[p1] = PortOpts{GM: true}
	teOpts.Ports[p2] = PortOpts{}
	opts.SwTstamp = true
	// teOpts.Ports["eth21"] = PortOpts{GM: true}
	// teOpts.Ports["eth22"] = PortOpts{}

	// opts.DefineCommonFlags() // TODO: Use some init function instead
	opts.DestIp = "224.0.1.129" // TODO: 224.0.0.107 for pdelays
	opts.Mac = "01:1b:19:00:00:00"
	opts.RecordPackets = true
	opts.Validate()
	go RunTeMode(opts, teOpts, app)
}

func WebServer() {

	// wsOut := make(chan []byte, 1000)
	app := NewApp()

	http.Handle("/", http.FileServer(http.FS(content)))
	http.HandleFunc("/ws", wsHandler(app))
	http.HandleFunc("/start", toggle(app, true))
	http.HandleFunc("/stop", toggle(app, false))

	fmt.Printf("Serving on http://localhost:8080\n")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
