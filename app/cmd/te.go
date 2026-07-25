// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	. "ckeep/internal"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type TeOpts struct {
	Ports    map[string]PortOpts
	Interval uint32
	// Count       uint32
	Peertopeer  bool
	DelayRecord uint32
	Unicast     bool

	// Internal fields
	IntervalTime time.Duration
	server       *Port
	client       *Port
	Stats        Stats
	Capturing    bool
	HasStats     bool
	Running      bool
	Export       string
}

func TeMode() {
	teOpts, opts := InitTeOpts()

	opts.DefineCommonFlags()
	opts.AddModeOpt(opts.Mode, &teOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt(opts.Mode, &teOpts.Peertopeer, 'P', "p2p", "", "Use P2P mode")
	// opts.AddModeOpt(mode, &teOpts.Count, 'c', "count", "<num>", "Number of packets to transmit. 0=infinite")
	opts.AddModeOpt(opts.Mode, &teOpts.DelayRecord, 'D', "delay_record", "<seconds>", "Time to wait until recording starts. Default: 0 seconds")
	opts.AddModeOpt(opts.Mode, &teOpts.Export, 'e', "export", "<filename>", "Export data to file")
	opts.AddModeOpt(opts.Mode, &teOpts.Unicast, 'U', "unicast", "", "Run in L4 unicast mode")

	if !opts.ParseFile(&teOpts) {
		return
	}
	if !opts.Parse() {
		return
	}

	err := ValidateTeOpts(&teOpts, &opts)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	app := NewApp(opts, false, true)
	RunTeMode(&teOpts, app)
}
func InitTeOpts() (TeOpts, CommonOpts) {
	var teOpts = TeOpts{}
	var opts = CommonOpts{Mode: "te"}
	opts.InitDefaults()
	teOpts.Interval = uint32(1000)
	teOpts.DelayRecord = uint32(0)

	teOpts.Ports = make(map[string]PortOpts)
	return teOpts, opts
}

func TeGetPortnames(teOpts *TeOpts, opts *CommonOpts) (string, string, error) {
	missingIp := false
	gmCount := 0
	p1name := ""
	p2name := ""
	for name, port := range teOpts.Ports {
		if opts.Udp && port.IP == "" {
			fmt.Printf("Missing IP on port %s\n", name)
			missingIp = true
		}
		if port.GM {
			p1name = name
			gmCount += 1
		} else {
			p2name = name
		}
	}
	if missingIp {
		return "", "", fmt.Errorf("Missing IP on ports")
	}
	if gmCount != 1 {
		return "", "", fmt.Errorf("Missing GM port")
	}
	return p1name, p2name, nil
}

func ValidateTeOpts(teOpts *TeOpts, opts *CommonOpts) error {
	err := opts.Validate()
	if err != nil {
		return err
	}

	if len(teOpts.Ports) != 2 {
		return fmt.Errorf("Two ports are required\n")
	}
	teOpts.IntervalTime = time.Duration(teOpts.Interval) * time.Millisecond
	// TODO: Validate port tstamp modes. facebook/time has some helpers.
	// Move out the above part to a validation function?

	p1name, p2name, err := TeGetPortnames(teOpts, opts)
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

	p1 := teOpts.Ports[p1name]
	p2 := teOpts.Ports[p2name]
	ip1 := net.ParseIP(p1.IP)
	ip2 := net.ParseIP(p2.IP)

	server := &Port{
		IfaceStr: p1name,
		IP:       ip1,
		DestIP:   ip2,
		PortOpts: p1,
	}
	client := &Port{
		IfaceStr: p2name,
		IP:       ip2,
		DestIP:   ip1,
		PortOpts: p2,
	}
	if teOpts.Unicast {
		opts.DestIp = ""
	} else { // This overrides the Port DestIP on Init()
		opts.DestIp = "224.0.1.129"
	}
	teOpts.server = server
	teOpts.client = client
	return nil
}

func error_exit(teOpts *TeOpts, app *App) {
	teOpts.HasStats = false
	log.Printf("Failed to start TE Mode")
	if app.Out != nil {
		app.Out <- []byte("exited")
	}
}

func RunTeMode(teOpts *TeOpts, app *App) {
	server := teOpts.server
	client := teOpts.client

	if !app.Cli {
		// In Web we only want to capture client. Maybe this should be
		// configurable later.
		server.Silent = true
	}
	teOpts.HasStats = false

	// Port1 is always GM
	app.Opts.Ip = server.PortOpts.IP
	app.Opts.DestIp = client.PortOpts.IP
	err := server.Init(app, 1)
	if err != nil {
		server.Deinit()
		error_exit(teOpts, app)
		return
	}

	app.Opts.Ip = teOpts.client.PortOpts.IP
	app.Opts.DestIp = teOpts.server.PortOpts.IP
	err = client.Init(app, 1)
	if err != nil {
		server.Deinit()
		client.Deinit()
		error_exit(teOpts, app)
		return
	}

	sigs := make(chan os.Signal, 10)
	if app.Cli {
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	}
	serverQuit := make(chan int)
	clientQuit := make(chan int)
	serverRx := make(chan PacketData, 100)
	clientRx := make(chan PacketData, 100)
	serverTicker := time.NewTicker(teOpts.IntervalTime)
	clientTicker := time.NewTicker(teOpts.IntervalTime)
	running := true

	delayRecordTimer := time.NewTimer(time.Duration(teOpts.DelayRecord) * time.Second)
	if teOpts.DelayRecord != 0 {
		server.RecordPackets = false
		client.RecordPackets = false
	} else {
		delayRecordTimer.Stop()
	}

	// TODO: Bad file descriptor after seqid ~280. Seems to be an UDP problem

	if !app.Cli {
		log.Printf("Starting TE Mode")
	}
	teOpts.Running = true
	go server.RxMode(serverRx, serverQuit)
	go client.RxMode(clientRx, clientQuit)
	for running {
		select {
		case <-sigs:
			serverQuit <- 0
			clientQuit <- 0
			running = false
			signal.Stop(sigs)
			// TODO: Temporary while developing
			// app.Out <- []byte("exit")
		case pd := <-serverRx:
			// fmt.Printf("Server rx\n")
			// pd.Print()
			if !teOpts.Peertopeer && pd.IsDelayReq() {
				server.ReplyToDelayReq(&pd)
			}
			if teOpts.Peertopeer && pd.IsPDelayReq() {
				server.ReplyToPDelayReq(&pd)
			}
		case pd := <-clientRx:
			// fmt.Printf("Client rx\n")
			// client.ShowPacket(pd)
			if teOpts.Peertopeer && pd.IsPDelayReq() {
				client.ReplyToPDelayReq(&pd)
			}
		case <-serverTicker.C:
			// TODO: Add websocket to transmit
			server.TransmitAnnounce()
			server.TransmitSyncFup()
			if teOpts.Peertopeer {
				server.TransmitPDelayReq()
			}
			// XXX: Something is going on with RX timestamps. Dagger bug?
			// Issue seems to be resolved if we run Deinit()
			// properly to disable timestamping.
		case <-clientTicker.C:
			// TODO: Add websocket to transmit
			if teOpts.Peertopeer {
				client.TransmitPDelayReq()
			} else {
				client.TransmitDelayReq()
			}
		case <-delayRecordTimer.C:
			str1 := "========================"
			str2 := " Starting recording "
			str3 := "========================\n"
			fmt.Print(str1 + str2 + str3)
			server.RecordPackets = true
			client.RecordPackets = true
		case msg := <-app.In:
			str := string(msg)
			switch str {
			case "exit":
				serverQuit <- 0
				clientQuit <- 0
				running = false
				signal.Stop(sigs)
			case "record":
				client.EnableRecording()
			}
		}
	}
	server.Deinit()
	client.Deinit()

	if app.Cli {
		fmt.Printf("\n")
	}
	if teOpts.Peertopeer {
		stats := client.GetP2pTE()
		if app.Cli {
			fmt.Printf("Mean T1: %d\n", stats.T1M.Mean)
			fmt.Printf("Mean Pdelay: %d\n", stats.PDelayM.Mean)
			fmt.Printf("Mean FwdAcc: %d\n", stats.FwdAccM.Mean)
		}
		stats.GenerateFile(false, teOpts.Export)
		teOpts.Stats = stats
		teOpts.HasStats = true
		if app.Out != nil {
			app.Out <- []byte("exited")
		}
	} else {
		stats := client.GetE2eTE()
		if app.Cli {
			fmt.Printf("Mean T1: %d\n", stats.T1M.Mean)
			fmt.Printf("Mean T4: %d\n", stats.T4M.Mean)
			fmt.Printf("Mean 2Way: %d\n", stats.TwowayM.Mean)
		}

		stats.GenerateFile(false, teOpts.Export)
		teOpts.Stats = stats
		teOpts.HasStats = true
		if app.Out != nil {
			app.Out <- []byte("exited")
		}
	}
	if !app.Cli {
		log.Printf("Exiting TE Mode")
	}
}
