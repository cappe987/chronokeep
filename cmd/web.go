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

//go:embed index.html
//go:embed style.css
//go:embed static/htmx.min.js
//go:embed static/ws.min.js
var content embed.FS

func wsHandler(wsOut chan []byte) http.HandlerFunc {
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

			case msg := <-wsOut:
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

func WebServer() {

	wsOut := make(chan []byte, 1000)

	var teOpts = TeOpts{}
	var opts = CommonOpts{Mode: "te"}
	// teOpts.Count = 1
	teOpts.Interval = uint32(1000)
	teOpts.DelayRecord = uint32(0)
	opts.Iface = "dummy"
	opts.Ip = "dummy"

	teOpts.Ports = make(map[string]PortOpts)
	teOpts.Ports["veth1"] = PortOpts{GM: true}
	teOpts.Ports["veth2"] = PortOpts{}
	opts.SwTstamp = true
	// teOpts.Ports["eth21"] = PortOpts{GM: true}
	// teOpts.Ports["eth22"] = PortOpts{}

	opts.DefineCommonFlags()
	opts.Validate()
	go RunTeMode(opts, teOpts, wsOut)

	http.Handle("/", http.FileServer(http.FS(content)))
	http.HandleFunc("/ws", wsHandler(wsOut))

	fmt.Printf("Serving on http://localhost:8080\n")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
