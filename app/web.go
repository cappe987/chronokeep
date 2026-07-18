//go:build !noweb

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>

package app

import (
	"context"
	"embed"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"time"

	. "ckeep/app/cmd"
	. "ckeep/internal"

	ptp "github.com/cappe987/facebook-time/ptp/protocol"

	"github.com/coder/websocket"
)

//go:embed static/*
var content embed.FS

type WebApp struct {
	App    *App
	TeOpts *TeOpts
	Tmpl   map[string]*template.Template
}

func buildHtmxPacket(pd PacketData) string {
	msgtype := pd.Packet.MessageType()
	hdr := pd.GetHeader()
	rx_ns := pd.HwTstamp.UnixNano() % 1000000000
	rx_s := pd.HwTstamp.Unix()
	seq := hdr.SequenceID
	corr := hdr.CorrectionField.Duration()
	domain := hdr.DomainNumber
	iface := fmt.Sprintf("%6s", pd.Iface)

	var originTs ptp.Timestamp
	ots_s := int64(0)
	ots_ns := int64(0)
	switch msgtype {
	case ptp.MessageSync:
		originTs = pd.GetSyncOriginTimestamp()
		ots_s = originTs.Nano() / 1000000000
		ots_ns = originTs.Nano() % 1000000000
	case ptp.MessageFollowUp:
		originTs = pd.GetFupOriginTimestamp()
		ots_s = originTs.Nano() / 1000000000
		ots_ns = originTs.Nano() % 1000000000
	case ptp.MessageDelayResp:
		originTs = pd.GetDelayRespOriginTimestamp()
		ots_s = originTs.Nano() / 1000000000
		ots_ns = originTs.Nano() % 1000000000
	case ptp.MessagePDelayResp:
		originTs = pd.GetPDelayRespRequestReceiptTimestamp()
		ots_s = originTs.Nano() / 1000000000
		ots_ns = originTs.Nano() % 1000000000
	case ptp.MessagePDelayRespFollowUp:
		originTs = pd.GetPDelayRespFupResponseOriginTimestamp()
		ots_s = originTs.Nano() / 1000000000
		ots_ns = originTs.Nano() % 1000000000
	}

	rxtx_str := "RX"
	if pd.IsTx {
		rxtx_str = "TX"
	}

	return fmt.Sprintf(`
<tbody hx-swap-oob="beforeend:#msgs">
<tr>
    <td>%s</td>
    <td>%s</td>
    <td>%s</td>
    <td>%d</td>
    <td>%d</td>
    <td>%d</td>
    <td>%d.%09d</td>
    <td>%d.%09d</td>
</tr>
</tbody>
`, iface, rxtx_str, msgtype, domain, seq, corr, ots_s, ots_ns, rx_s, rx_ns)
}

func wsHandler(wa *WebApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

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

		// Writer loop
		for {
			select {
			case <-ctx.Done():
				conn.Close(websocket.StatusNormalClosure, "")
				return

			case pd := <-wa.App.WsOut:
				// TODO: Add command to start recording manually
				msg := buildHtmxPacket(pd)
				// if string(msg) == "exit" {
				// 	conn.Close(websocket.StatusNormalClosure, "")
				// 	return
				// }
				writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				// log.Printf("WS received from channel: %s\n", msg)
				err := conn.Write(writeCtx, websocket.MessageText, []byte(msg))
				cancel()

				if err != nil {
					return
				}
			}
		}
	}
}

// TODO: CLean up this function
func toggle(wa *WebApp) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Invalid request method.", 405)
		}
		// fmt.Fprintf(w, "GET, %q", html.EscapeString(r.URL.Path))
		// log.Printf("POST request")

		action := r.PostFormValue("action")
		swtstamp := r.PostFormValue("software")
		peertopeer := r.PostFormValue("peertopeer")
		p1 := r.PostFormValue("port1")
		p2 := r.PostFormValue("port2")
		domain := r.PostFormValue("domain")
		tagged := r.PostFormValue("vlan-tagged")
		vid := r.PostFormValue("vlan")
		prio := r.PostFormValue("prio")
		interval := r.PostFormValue("interval")
		// log.Printf("Action: %s\n", action)
		// log.Printf("Swt: %s\n", swtstamp)
		// log.Printf("p2p: %s\n", peertopeer)
		// log.Printf("domain: %s\n", domain)
		// log.Printf("tagged: %s\n", tagged)
		// log.Printf("vid: %s\n", vid)
		// log.Printf("prio: %s\n", prio)

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
		interval_ms, err := strconv.Atoi(interval)
		if err != nil {
			fmt.Printf("Invalid interval\n")
		}
		wa.TeOpts.Interval = uint32(interval_ms)

		wa.TeOpts.Ports[p1] = PortOpts{GM: true}
		wa.TeOpts.Ports[p2] = PortOpts{}
		if p1 == p2 {
			return
		}

		if !ValidateTeOpts(wa.TeOpts, &wa.App.Opts) {
			return
		}

		starting := false
		exiting := false
		if action == "start" {
			wa.TeOpts.Running = true
			starting = true
			start_app(wa, false)
		} else if action == "start-and-capture" {
			starting = true
			wa.TeOpts.Running = true
			wa.TeOpts.Capturing = true
			start_app(wa, true)
		} else if action == "record" {
			wa.TeOpts.Capturing = true
			if wa.TeOpts.Running {
				wa.App.In <- []byte(action)
			}
		} else {
			if wa.TeOpts.Running {
				exiting = true
				wa.App.In <- []byte("exit")
				msg := string(<-wa.App.Out)
				if msg != "exited" {
					fmt.Printf("Expected mode to exit\n")
					return
				}
				wa.TeOpts.Running = false
				wa.TeOpts.Capturing = false
			}
		}

		td := make(map[string]any)
		td["TeRunning"] = wa.TeOpts.Running
		td["Capturing"] = wa.TeOpts.Capturing
		td["Starting"] = starting
		td["Exiting"] = exiting
		stats := wa.TeOpts.Stats
		if exiting {
			td["Stats"] = stats.GetWebStats(wa.TeOpts.Peertopeer)
		}
		err = wa.Tmpl["buttons"].Execute(w, td)
		if err != nil {
			fmt.Printf("Error executing index.html template: %s\n", err)
		}

	}
}

func start_app(wa *WebApp, capture bool) {
	wa.App.Opts.RecordPackets = capture
	go RunTeMode(wa.TeOpts, wa.App)
}

func fill_page_data(wa *WebApp, td map[string]any, mode, title string) {
	td["Mode"] = mode
	td["ModeTitle"] = title
	td["TeRunning"] = wa.TeOpts.Running
}

func te_page(wa *WebApp, td map[string]any) {
	fill_page_data(wa, td, "time-error", "Time Error Mode")
	td["TeOpts"] = *wa.TeOpts
	td["Opts"] = wa.App.Opts
	td["AllPorts"] = GetSystemPorts()
	// TODO: handle errors for TeGetPortnames
	p1name, p2name, _ := TeGetPortnames(wa.TeOpts, &wa.App.Opts)
	td["Port1"] = p1name // Port 1 is always GM
	td["Port2"] = p2name
	td["Capturing"] = wa.TeOpts.Capturing
	if wa.TeOpts.HasStats {
		td["HasStats"] = true
		td["Stats"] = wa.TeOpts.Stats.GetWebStats(wa.TeOpts.Peertopeer)
	}
}

func help_page(wa *WebApp, td map[string]any) {
	fill_page_data(wa, td, "help", "Help")
}

func packet_page(wa *WebApp, td map[string]any) {
	fill_page_data(wa, td, "packet", "Packet Mode")
}

func serve_page(wa *WebApp) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		page := r.PostFormValue("page")
		td := make(map[string]any)
		switch page {
		case "te":
			te_page(wa, td)
		case "help":
			help_page(wa, td)
		case "packet":
			packet_page(wa, td)
		default:
			return
		}
		td["DoOob"] = true
		err := wa.Tmpl[page].Execute(w, td)
		if err != nil {
			fmt.Printf("Error executing index.html template: %s\n", err)
		}
	}
}

func serve_index(wa *WebApp) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		td := make(map[string]any)
		switch html.EscapeString((r.URL.Path)) {
		case "/":
			te_page(wa, td)
		case "/index.html":
			te_page(wa, td)
		case "/packet.html":
			packet_page(wa, td)
		case "/help.html":
			help_page(wa, td)
		}
		err := wa.Tmpl["index.html"].Execute(w, td)
		if err != nil {
			fmt.Printf("Error executing index.html template: %s\n", err)
		}
	}
}

func WebServer() {
	teOpts, opts := InitTeOpts()
	if !opts.ParseFile(&teOpts) {
		fmt.Printf("Failed parsing file\n")
		return
	}
	app := NewApp(opts, true, false)
	webapp := WebApp{App: app, TeOpts: &teOpts}
	tmpl, err := BuildTemplates()
	if err != nil {
		fmt.Printf("Error parsing templates: %s\n", err)
		return
	}
	webapp.Tmpl = tmpl

	static_content, err := fs.Sub(content, "static")
	if err != nil {
		fmt.Printf("Error loading static content: %s\n", err)
		return
	}
	handler := http.StripPrefix("/static/", http.FileServer(http.FS(static_content)))
	http.Handle("/static/", handler)
	http.HandleFunc("/ws", wsHandler(&webapp))
	http.HandleFunc("/te-toggle", toggle(&webapp))
	http.HandleFunc("/serve-page", serve_page(&webapp))
	http.HandleFunc("/", serve_index(&webapp))

	fmt.Printf("Serving on http://localhost:8080\n")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
