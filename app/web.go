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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "ckeep/app/cmd"
	. "ckeep/internal"

	ptp "github.com/cappe987/facebook-time/ptp/protocol"
	"github.com/pborman/getopt/v2"

	"github.com/coder/websocket"
)

//go:embed static/*
var content embed.FS

type WebApp struct {
	App    *App
	TeOpts *TeOpts
	WtOpts *WtOpts
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
		if !originTs.Empty() {
			tm := originTs.Time()
			ots_s = tm.Unix()
			ots_ns = int64(tm.Nanosecond())
		}
	case ptp.MessageFollowUp:
		originTs = pd.GetFupOriginTimestamp()
		if !originTs.Empty() {
			tm := originTs.Time()
			ots_s = tm.Unix()
			ots_ns = int64(tm.Nanosecond())
		}
	case ptp.MessageDelayResp:
		originTs = pd.GetDelayRespOriginTimestamp()
		if !originTs.Empty() {
			tm := originTs.Time()
			ots_s = tm.Unix()
			ots_ns = int64(tm.Nanosecond())
		}
	case ptp.MessagePDelayResp:
		originTs = pd.GetPDelayRespRequestReceiptTimestamp()
		if !originTs.Empty() {
			tm := originTs.Time()
			ots_s = tm.Unix()
			ots_ns = int64(tm.Nanosecond())
		}
	case ptp.MessagePDelayRespFollowUp:
		originTs = pd.GetPDelayRespFupResponseOriginTimestamp()
		if !originTs.Empty() {
			tm := originTs.Time()
			ots_s = tm.Unix()
			ots_ns = int64(tm.Nanosecond())
		}
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
			LogError("accept error: %s", err)
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

func send_buttons_error(td map[string]any, w http.ResponseWriter, wa *WebApp, err error) {
	td["Error"] = err
	send_buttons(td, w, wa)
}

func send_buttons(td map[string]any, w http.ResponseWriter, wa *WebApp) {
	td["TeRunning"] = wa.TeOpts.Running
	td["Capturing"] = wa.TeOpts.Capturing
	err := wa.Tmpl["buttons"].Execute(w, td)
	if err != nil {
		fmt.Printf("Error executing buttons template: %s\n", err)
	}
}

func (wa *WebApp) parse_domain(value string) error {
	if value == "" {
		return fmt.Errorf("Missing domain")
	} else {
		domain, err := strconv.Atoi(value)
		if err != nil {
			// ... handle error
			return fmt.Errorf("Invalid domain")
		} else {
			wa.App.Opts.Domain = uint8(domain)
		}
	}
	return nil
}

func (wa *WebApp) parse_vlan(tagged, vid, prio string) error {
	if tagged == "" {
		wa.App.Opts.Vlan = nil
	} else {
		i1, err1 := strconv.Atoi(vid)
		if err1 != nil {
			return fmt.Errorf("Missing or invalid VLAN")
		} else {
			v := uint16(i1)
			wa.App.Opts.Vlan = &v
		}
		i2, err2 := strconv.Atoi(prio)
		if err2 != nil {
			return fmt.Errorf("Missing or invalid Prio")
		} else {
			wa.App.Opts.Prio = uint8(i2)
		}
	}
	return nil
}

func (wa *WebApp) parse_ports(p1, p2 string) {
	// Preserve settings for port. E.g. ingr/egr latency since that isn't
	// exposed. When latency is implemented in TE, it can be set via config
	// file even if web doesn't support it.
	newports := make(map[string]PortOpts)
	val, ok := wa.TeOpts.Ports[p1]
	if ok {
		val.GM = true
		newports[p1] = val
	} else {
		newports[p1] = PortOpts{GM: true}
	}
	val, ok = wa.TeOpts.Ports[p2]
	if ok {
		val.GM = false
		newports[p2] = val
	} else {
		newports[p2] = PortOpts{}
	}
	wa.TeOpts.Ports = newports
}

func (wa *WebApp) parse_settings(r *http.Request) error {
	p1 := r.PostFormValue("port1")
	p2 := r.PostFormValue("port2")
	domain := r.PostFormValue("domain")
	tagged := r.PostFormValue("vlan-tagged")
	vid := r.PostFormValue("vlan")
	prio := r.PostFormValue("prio")
	interval := r.PostFormValue("interval")

	wa.App.Opts.SwTstamp = r.PostFormValue("software") == "on"
	wa.App.Opts.Onestep = r.PostFormValue("onestep") == "on"
	wa.TeOpts.Peertopeer = r.PostFormValue("peertopeer") == "on"
	err := wa.parse_domain(domain)
	if err != nil {
		return err
	}
	err = wa.parse_vlan(tagged, vid, prio)
	if err != nil {
		return err
	}
	interval_ms, err := strconv.Atoi(interval)
	if err != nil {
		return fmt.Errorf("Invalid interval")
	}
	wa.TeOpts.Interval = uint32(interval_ms)
	wa.parse_ports(p1, p2)

	err = ValidateTeOpts(wa.TeOpts, &wa.App.Opts)
	if err != nil {
		return err
	}
	return nil
}

func toggle(wa *WebApp) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Invalid request method.", 405)
		}
		td := make(map[string]any)
		action := r.PostFormValue("action")

		switch action {
		case "start":
			err := wa.parse_settings(r)
			if err != nil {
				send_buttons_error(td, w, wa, err)
				return
			}
			wa.TeOpts.Running = true
			td["Starting"] = true
			start_app(wa, false)
		case "start-and-capture":
			err := wa.parse_settings(r)
			if err != nil {
				send_buttons_error(td, w, wa, err)
				return
			}
			td["Starting"] = true
			wa.TeOpts.Running = true
			wa.TeOpts.Capturing = true
			start_app(wa, true)
		case "record":
			wa.TeOpts.Capturing = true
			if wa.TeOpts.Running {
				wa.App.In <- []byte(action)
			}
		case "stop":
			if !wa.TeOpts.Running {
				break
			}
			td["Exiting"] = true
			wa.App.In <- []byte("exit")
			msg := string(<-wa.App.Out)
			if msg != "exited" {
				fmt.Printf("Expected mode to exit\n")
				return
			}
			wa.TeOpts.Running = false
			wa.TeOpts.Capturing = false
			stats := wa.TeOpts.Stats
			td["Stats"] = stats.GetWebStats(wa.TeOpts.Peertopeer)
		}
		send_buttons(td, w, wa)
	}
}

func start_app(wa *WebApp, capture bool) {
	wa.App.Opts.RecordPackets = capture
	go RunTeMode(wa.TeOpts, wa.App)
}

func IsHxRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func fill_page_data(wa *WebApp, td map[string]any, mode, title string) {
	td["Mode"] = mode
	td["ModeTitle"] = title
	td["TeRunning"] = wa.TeOpts.Running
	td["WtRunning"] = wa.WtOpts.Running
}

func te_page(wa *WebApp, td map[string]any) {
	fill_page_data(wa, td, "time-error", "Time Error Mode")
	td["TeOpts"] = *wa.TeOpts
	td["Opts"] = wa.App.Opts
	td["AllPorts"] = GetSystemPorts()
	// TODO: handle errors for TeGetPortnames
	p1name, p2name, _ := GetPortnames(wa.TeOpts.Ports, &wa.App.Opts)
	td["Port1"] = p1name // Port 1 is always GM
	td["Port2"] = p2name
	td["Capturing"] = wa.TeOpts.Capturing
	if wa.TeOpts.HasStats {
		td["HasStats"] = true
		td["Stats"] = wa.TeOpts.Stats.GetWebStats(wa.TeOpts.Peertopeer)
	}
}

func wiretime_page(wa *WebApp, td map[string]any) {
	fill_page_data(wa, td, "wiretime", "Wiretime")
}

func help_page(wa *WebApp, td map[string]any) {
	fill_page_data(wa, td, "help", "Help")
}

func examples_page(wa *WebApp, td map[string]any) {
	fill_page_data(wa, td, "examples", "Examples")
}

func packet_page(wa *WebApp, td map[string]any) {
	fill_page_data(wa, td, "packet", "Packet Mode")
}

func serve_index(wa *WebApp) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		td := make(map[string]any)
		url := html.EscapeString((r.URL.Path))
		switch url {
		case "/": // Default to TE page
			url = "/te.html"
			te_page(wa, td)
		case "/te.html":
			te_page(wa, td)
		case "/packet.html":
			packet_page(wa, td)
		case "/help.html":
			help_page(wa, td)
		case "/examples.html":
			examples_page(wa, td)
		case "/wiretime.html":
			wiretime_page(wa, td)
		default:
			return
		}
		url = url[1:]

		if IsHxRequest(r) {
			url = strings.TrimSuffix(url, filepath.Ext(url))
			td["DoOob"] = true
			err := wa.Tmpl[url].Execute(w, td)
			if err != nil {
				fmt.Printf("Error executing '%s' template: %s\n", url, err)
			}
		} else {
			err := wa.Tmpl["index"].Execute(w, td)
			if err != nil {
				fmt.Printf("Error executing '%s' template: %s\n", url, err)
			}
		}
	}
}

func WebServer() {
	mode := "web"
	opts := CommonOpts{Mode: mode}
	opts.DefineCommonLimited()
	err := getopt.Getopt(nil)
	if err != nil {
		opts.Usage()
		LogError("%s\n", err)
		return
	}
	opts.SetLogLevel()
	if opts.Help {
		opts.Usage()
		return
	}
	teOpts, opts := InitTeOpts()
	if !opts.ParseFile(&teOpts) {
		LogError("Failed parsing file\n")
		return
	}
	wtOpts, opts := InitWiretimeOpts(true)
	app := NewApp(opts, true, false)
	webapp := WebApp{App: app, TeOpts: &teOpts, WtOpts: &wtOpts}
	tmpl, err := BuildTemplates()
	if err != nil {
		LogError("Parsing templates: %s\n", err)
		return
	}
	webapp.Tmpl = tmpl

	static_content, err := fs.Sub(content, "static")
	if err != nil {
		LogError("Loading static content: %s\n", err)
		return
	}
	handler := http.StripPrefix("/static/", http.FileServer(http.FS(static_content)))
	http.Handle("/static/", handler)
	http.HandleFunc("/ws", wsHandler(&webapp))
	http.HandleFunc("/te-toggle", toggle(&webapp))
	http.HandleFunc("/", serve_index(&webapp))

	LogNotice("Serving on http://localhost:%d", 8080)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
