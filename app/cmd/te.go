// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	. "ckeep/internal"
	"fmt"
	"net"
	"sync"
	"time"
)

type TeOpts struct {
	Ports    map[string]PortOpts
	Interval uint32
	// Count       uint32
	Peertopeer  bool
	DelayRecord uint32
	Unicast     bool
	Export      string

	// Internal fields
	IntervalTime time.Duration
	server       *Port
	client       *Port
	Stats        Stats
	Capturing    bool
	HasStats     bool
	Running      bool
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

	p1name, p2name, err := GetPortnames(teOpts.Ports, opts)
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
		Name:     p1name,
		IP:       ip1,
		DestIP:   ip2,
		PortOpts: p1,
	}
	client := &Port{
		Name:     p2name,
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
	LogError("Failed to start TE Mode")
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

	serverCfg := Config{ReplyPDelay: true, ReplyDelay: true, TxSyncInterval: teOpts.IntervalTime}
	var clientCfg Config
	if teOpts.Peertopeer {
		clientCfg = Config{TxPDelayInterval: teOpts.IntervalTime}
	} else {
		clientCfg = Config{TxDelayInterval: teOpts.IntervalTime}
	}

	app.EnableSignals()
	var wg sync.WaitGroup
	wg.Add(2)
	// TODO: Handle error reporting
	go server.RunPort(app, serverCfg, &wg)
	go client.RunPort(app, clientCfg, &wg)

	select {
	case <-app.Sigs:
	case <-app.QuitCh:
	}
	server.Exit()
	client.Exit()
	wg.Wait()
	app.DisableSignals()

	if app.Cli && !server.Silent {
		fmt.Printf("\n")
	}
	if teOpts.Peertopeer {
		stats := client.GetP2pTE()
		if app.Cli {
			fmt.Fprintf(app.W, "Mean T1: %d\n", stats.T1M.Mean)
			fmt.Fprintf(app.W, "Mean Pdelay: %d\n", stats.PDelayM.Mean)
			fmt.Fprintf(app.W, "Mean FwdAcc: %d\n", stats.FwdAccM.Mean)
		}
		stats.GenerateFile(false, teOpts.Export)
		teOpts.Stats = stats
		teOpts.HasStats = true
	} else {
		stats := client.GetE2eTE()
		if app.Cli {
			fmt.Fprintf(app.W, "Mean T1: %d\n", stats.T1M.Mean)
			fmt.Fprintf(app.W, "Mean T4: %d\n", stats.T4M.Mean)
			fmt.Fprintf(app.W, "Mean 2Way: %d\n", stats.TwowayM.Mean)
		}

		stats.GenerateFile(false, teOpts.Export)
		teOpts.Stats = stats
		teOpts.HasStats = true
	}
	if app.Out != nil {
		app.Out <- []byte("exited")
	}
	if !app.Cli {
		LogNotice("Exiting TE Mode on ports (%s, %s)", server.Name, client.Name)
	}
}
