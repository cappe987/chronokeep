// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	. "ckeep/internal"
	"sync"
	"time"

	ptp "github.com/cappe987/facebook-time/ptp/protocol"
	"github.com/pborman/getopt/v2"
)

type PktOpts struct {
	// Configuration file
	Ports    map[string]PortOpts
	Interval uint32
	Count    uint32
	Seq      uint16
	Sequence []string
	RxMode   bool
	Vlan     int
	Prio     int
	// ClkType   string

	// Internal fields
	SequenceTypes []ptp.MessageType
	IntervalTime  time.Duration

	// TODO: time/timestamp needs to be patched to support this.
	// enum delay_mechanism dm;
	// enum hwtstamp_clk_types clk_type;
	// header_offset     uint
}

func (pktOpts *PktOpts) InitDefaults() {
	pktOpts.Count = 1
	pktOpts.Interval = uint32(1000)
	pktOpts.Vlan = -1
	pktOpts.Prio = -1
}

func PktMode() {
	mode := "pkt"
	var opts = CommonOpts{Mode: mode}
	var pktOpts = PktOpts{}
	pktOpts.InitDefaults()
	noWait := false
	opts.InitDefaults()

	opts.DefineCommonFlags()
	opts.AddModeOpt(mode, &pktOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt(mode, &pktOpts.Seq, 's', "seq", "<seqid>", "Starting SequenceID")
	opts.AddModeOpt(mode, &pktOpts.RxMode, 'r', "rx", "", "Receive only")
	opts.AddModeOpt(mode, &pktOpts.Count, 'c', "count", "<num>", "Number of packets to transmit. 0=infinite")
	opts.AddModeOpt(mode, &pktOpts.Vlan, 'V', "vlan", "<VID>", "VLAN ID")
	opts.AddModeOpt(mode, &pktOpts.Prio, 'p', "prio", "<PRIO>", "VLAN Prio")

	if !opts.ParseFile(&pktOpts) {
		return
	}
	if !opts.Parse() {
		return
	}

	////////////////////////////////////////////////////
	if pktOpts.Interval == 0 {
		noWait = true
	} else {
		pktOpts.IntervalTime = time.Duration(pktOpts.Interval) * time.Millisecond
	}
	pkttypes := getopt.Args()
	if len(pkttypes) == 0 {
		pkttypes = pktOpts.Sequence
	}
	for _, str := range pkttypes {
		pktOpts.SequenceTypes = append(pktOpts.SequenceTypes, StringToMessageType(str))
	}

	pname := ""
	var popts PortOpts
	for name, port := range pktOpts.Ports {
		pname = name
		popts = port
		break
	}

	if opts.Iface != "dummy" {
		pname = opts.Iface
	}
	if pname == "" {
		LogError("No port selected\n")
		return
	}

	err := opts.ValidateTstampMode(pname)
	if err != nil {
		LogError("%s\n", err)
		return
	}

	port := Port{
		Name: pname,
		PortOpts: PortOpts{
			IP:     opts.Ip,
			DestIP: opts.DestIp,
		},
		// TODO: Fix IP parsing from file
		// IP:       ip2,
		// DestIP:   ip1,
	}
	opts.Iface = pname
	opts.Vlan = popts.Vlan
	opts.Prio = popts.Prio
	if pktOpts.Vlan >= 0 {
		vid := uint16(pktOpts.Vlan)
		opts.Vlan = &vid
	}
	if pktOpts.Prio >= 0 {
		opts.Prio = uint8(pktOpts.Prio)
		if opts.Vlan == nil {
			vid := uint16(0)
			opts.Vlan = &vid
		}
	}

	app := NewApp(opts, false, true)

	///////////////////////////////////////////////////
	runPktMode(app, &port, &pktOpts, noWait)

}

func runPktMode(app *App, port *Port, pktOpts *PktOpts, noWait bool) {
	cfg := Config{
		Count:         &pktOpts.Count,
		SequenceTypes: pktOpts.SequenceTypes,
		RxOnly:        pktOpts.RxMode,
	}
	if !cfg.RxOnly { // RxOnly takes prededence over NoWait
		cfg.NoWait = noWait
		cfg.TxSequenceInterval = pktOpts.IntervalTime
	}

	app.EnableSignals()
	var wg sync.WaitGroup
	wg.Add(1)
	done := make(chan bool)
	go func() {
		port.RunPort(app, cfg, &wg, nil)
		done <- true
	}()

	// Wait for signal, app exit signal, or port is finished
	select {
	case <-app.Sigs:
	case <-app.QuitCh:
	case <-done:
	}
	port.Exit()
	wg.Wait()
	app.DisableSignals()
}
