// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	. "ckeep/internal"
	"sync"
	"time"
)

type gmOpts struct {
	Interval   uint32
	Peertopeer bool
}

func GmMode() {
	var port Port
	var opts = CommonOpts{Mode: "gm"}
	var gmOpts = gmOpts{}
	interval_ms := uint32(1000)
	opts.InitDefaults()

	opts.DefineCommonFlags()
	opts.RecordPackets = false
	opts.AddModeOpt("gm", &gmOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt("gm", &gmOpts.Peertopeer, 'P', "peertopeer", "", "P2P Mode")

	if !opts.ParseFile(&gmOpts) {
		return
	}
	if !opts.Parse() {
		return
	}

	// TODO: Add PortOpts to gmOpts struct. Unify port parsing with pkt.go
	if opts.Iface == "" {
		LogError("No port selected")
		return
	}

	////////////////////////////////////////////////////

	Interval := time.Duration(interval_ms) * time.Millisecond

	app := NewApp(opts, false, true)
	serverCfg := Config{ReplyPDelay: true, ReplyDelay: true, TxSyncInterval: Interval, TxAnnoInterval: Interval}

	app.EnableSignals()
	var wg sync.WaitGroup
	wg.Add(1)
	go port.RunPort(app, serverCfg, &wg, nil)

	select {
	case <-app.Sigs:
	case <-app.QuitCh:
	}
	port.Exit()
	wg.Wait()
	app.DisableSignals()

}
