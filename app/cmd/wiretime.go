// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	. "ckeep/internal"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type WireTime struct {
	// Time on wire (latency)
	wiretime time.Duration
	// Relative tstamp
	tstamp time.Duration
	seq    uint16
}

type WtOpts struct {
	Ports    map[string]PortOpts
	Interval uint32
	Unicast  bool
	Export   string

	// Internal fields
	IntervalTime time.Duration
	server       *Port
	client       *Port
	Stats        Stats
	HasStats     bool
	Running      bool
	WsOut        chan string // Should we send WireTime instead of raw html?
}

func WiretimeMode() {
	wtOpts, opts := InitWiretimeOpts(false)

	opts.DefineCommonFlags()
	opts.AddModeOpt(opts.Mode, &wtOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt(opts.Mode, &wtOpts.Export, 'e', "export", "<filename>", "Export data to file")
	opts.AddModeOpt(opts.Mode, &wtOpts.Unicast, 'U', "unicast", "", "Run in L4 unicast mode")

	if !opts.ParseFile(&wtOpts) {
		return
	}
	if !opts.Parse() {
		return
	}

	err := ValidateWiretimeOpts(&wtOpts, &opts)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	app := NewApp(opts, false, true)
	RunWiretimeMode(&wtOpts, app)
}
func InitWiretimeOpts(init_channel bool) (WtOpts, CommonOpts) {
	var wtOpts = WtOpts{}
	var opts = CommonOpts{Mode: "te"}
	opts.InitDefaults()
	wtOpts.Interval = uint32(1000)

	wtOpts.Ports = make(map[string]PortOpts)
	if init_channel {
		wtOpts.WsOut = make(chan string, 100)
	}
	return wtOpts, opts
}

func ValidateWiretimeOpts(wtOpts *WtOpts, opts *CommonOpts) error {
	err := opts.Validate()
	if err != nil {
		return err
	}

	if len(wtOpts.Ports) != 2 {
		return fmt.Errorf("Two ports are required\n")
	}
	wtOpts.IntervalTime = time.Duration(wtOpts.Interval) * time.Millisecond
	p1name, p2name, err := GetPortnames(wtOpts.Ports, opts)
	if err != nil {
		return err
	}
	if p1name == p2name {
		return fmt.Errorf("Ports cannot be the same")
	}

	err = opts.ValidateTstampMode(p1name)
	if err != nil {
		return err
	}
	err = opts.ValidateTstampMode(p2name)
	if err != nil {
		return err
	}

	p1 := wtOpts.Ports[p1name]
	p2 := wtOpts.Ports[p2name]
	ip1 := net.ParseIP(p1.IP)
	ip2 := net.ParseIP(p2.IP)

	server := &Port{
		Name:     p1name,
		IP:       ip1,
		DestIP:   ip2,
		PortOpts: p1,
		Silent:   true,
	}
	client := &Port{
		Name:     p2name,
		IP:       ip2,
		DestIP:   ip1,
		PortOpts: p2,
		Silent:   true,
	}
	if wtOpts.Unicast {
		opts.DestIp = ""
	} else { // This overrides the Port DestIP on Init()
		opts.DestIp = "224.0.1.129"
	}
	wtOpts.server = server
	wtOpts.client = client
	return nil
}

func wiretime_error_exit(wtOpts *WtOpts, app *App) {
	wtOpts.HasStats = false
	LogError("Failed to start Wiretime Mode")
	if app.Out != nil {
		app.Out <- []byte("exited")
	}
}

func RunWiretimeMode(wtOpts *WtOpts, app *App) {
	server := wtOpts.server
	client := wtOpts.client

	if !app.Cli {
		// In Web we only want to capture client. Maybe this should be
		// configurable later.
		server.Silent = true
	}
	wtOpts.HasStats = false

	// Port1 is always GM
	app.Opts.Ip = server.PortOpts.IP
	app.Opts.DestIp = client.PortOpts.IP
	err := server.Init(app, 1)
	if err != nil {
		server.Deinit()
		wiretime_error_exit(wtOpts, app)
		return
	}

	app.Opts.Ip = wtOpts.client.PortOpts.IP
	app.Opts.DestIp = wtOpts.server.PortOpts.IP
	err = client.Init(app, 1)
	if err != nil {
		server.Deinit()
		client.Deinit()
		wiretime_error_exit(wtOpts, app)
		return
	}

	sigs := make(chan os.Signal, 10)
	if app.Cli {
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	}
	serverRx := make(chan PacketData, 100)
	clientRx := make(chan PacketData, 100)
	serverTicker := time.NewTicker(wtOpts.IntervalTime)
	clientTicker := time.NewTicker(wtOpts.IntervalTime)
	running := true

	if !app.Cli {
		LogNotice("Starting Wiretime Mode on ports (%s, %s)", server.Name, client.Name)
	}
	wtOpts.Running = true
	server.StartRxMode(serverRx)
	client.StartRxMode(clientRx)
	txPkts := make(map[uint16]*PacketData)
	lats := make([]WireTime, 512)
	var firstRx *PacketData
	// var rxPkts map[uint16]*PacketData
	for running {
		select {
		case <-app.QuitCh:
			running = false
		case <-sigs:
			running = false
		case _ = <-serverRx:
		case rxpd := <-clientRx:
			if firstRx == nil {
				firstRx = &rxpd
			}
			txpd := hasMatch(txPkts, &rxpd)
			if txpd != nil {
				lat := getLatency(txpd, &rxpd)
				if client.App.Cli {
					fmt.Printf("Lat %d\n", lat)
				}
				wt := WireTime{
					tstamp:   rxpd.NormalizeSwtstamp(firstRx.SwTstamp),
					wiretime: lat,
					seq:      rxpd.GetSequenceID(),
				}
				lats = append(lats, wt)
				sendHtml(wtOpts, wt)
			}
		case <-serverTicker.C:
			txpd, err := server.TransmitSyncOnly()
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			savePacket(txPkts, txpd)
		case <-clientTicker.C:
		case msg := <-app.In:
			str := string(msg)
			switch str {
			case "exit":
				running = false
			}
		}
	}
	signal.Stop(sigs)
	server.Quit()
	client.Quit()

	if app.Cli && !server.Silent {
		fmt.Printf("\n")
	}
	// Calc stats here and output file

	// ===============
	if !app.Cli {
		LogNotice("Exiting TE Mode on ports (%s, %s)", server.Name, client.Name)
	}
}

func hasMatch(pkts map[uint16]*PacketData, pd *PacketData) *PacketData {
	seq := pd.GetSequenceID()
	match, ok := pkts[seq]
	if !ok {
		return nil
	}
	delete(pkts, seq)
	return match
}

func savePacket(pkts map[uint16]*PacketData, pd *PacketData) {
	seq := pd.GetSequenceID()
	pkts[seq] = pd
}

func getLatency(tx *PacketData, rx *PacketData) time.Duration {
	// TODO: Handle onestep?
	txts := tx.HwTstamp
	rxts := rx.HwTstamp
	return rxts.Sub(txts)
}

func sendHtml(wto *WtOpts, wt WireTime) {
	if wto.WsOut == nil {
		return
	}
	html := buildHtml(wt)
	wto.WsOut <- html
}

func buildHtml(wt WireTime) string {
	return fmt.Sprintf(`
		<p>Wiretime: %d</p>
		<p>Time: %d</p>
		`, wt.wiretime, wt.tstamp)
}
