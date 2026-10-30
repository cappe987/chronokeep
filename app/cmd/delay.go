// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	. "ckeep/internal"
	"fmt"
	"sync"
	"time"

	ptp "github.com/cappe987/facebook-time/ptp/protocol"
)

type DelayOpts struct {
	// Configuration file
	Ports      map[string]PortOpts
	Interval   uint32
	Count      uint32
	Seq        uint16
	Vlan       int
	Prio       int
	Client     bool
	Server     bool // Server replies to requests sent
	Peertopeer bool

	// Internal fields
	IntervalTime time.Duration
}

func (do *DelayOpts) InitDefaults() {
	do.Interval = uint32(1000)
	do.Vlan = -1
	do.Prio = -1
}

func DelayMode() {
	mode := "delay"
	var opts = CommonOpts{Mode: mode}
	var delayOpts = DelayOpts{}
	delayOpts.InitDefaults()
	opts.InitDefaults()

	opts.DefineCommonFlags()
	opts.AddModeOpt(mode, &delayOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt(mode, &delayOpts.Client, 'C', "client", "", "Run client")
	opts.AddModeOpt(mode, &delayOpts.Server, 's', "server", "", "Run server")
	opts.AddModeOpt(mode, &delayOpts.Vlan, 'V', "vlan", "<VID>", "VLAN ID")
	opts.AddModeOpt(mode, &delayOpts.Prio, 'p', "prio", "<PRIO>", "VLAN Prio")
	opts.AddModeOpt(mode, &delayOpts.Peertopeer, 'P', "peertopeer", "", "P2P mode")
	opts.AddModeOpt(mode, &delayOpts.Count, 'c', "count", "<num>", "Number of measurements to perform. Only for client.")

	if !opts.ParseFile(&delayOpts) {
		return
	}
	if !opts.Parse() {
		return
	}

	////////////////////////////////////////////////////

	// TODO: Allow setting interval 0 to skip using ticker
	delayOpts.IntervalTime = time.Duration(delayOpts.Interval) * time.Millisecond

	pname := ""
	var popts PortOpts
	for name, port := range delayOpts.Ports {
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

	port := Port{
		Name: pname,
		// TODO: Fix IP parsing from file
		// IP:       ip2,
		// DestIP:   ip1,
	}
	port.RecordPackets = false
	port.Silent = true
	opts.Iface = pname
	opts.Vlan = popts.Vlan
	opts.Prio = popts.Prio
	if delayOpts.Vlan >= 0 {
		vid := uint16(delayOpts.Vlan)
		opts.Vlan = &vid
	}
	if delayOpts.Prio >= 0 {
		opts.Prio = uint8(delayOpts.Prio)
		if opts.Vlan == nil {
			vid := uint16(0)
			opts.Vlan = &vid
		}
	}

	app := NewApp(opts, false, true)

	if delayOpts.Client {
		// TODO: allow setting zero
		if delayOpts.Count == 0 {
			LogError("No count selected\n")
			return
		}
		client(app, &delayOpts, &port)
	} else if delayOpts.Server {
		server(app, &delayOpts, &port)
	} else {
		LogError("Please select -c/--client or -s/--server\n")
	}
	port.Deinit()
}

type clientData struct {
	reqs     []PacketData
	resps    []PacketData
	respFups []PacketData
	count    uint32
	do       *DelayOpts
	app      *App
}

func process_packet(data *clientData, pd *PacketData, count *uint32, running *bool) {
	if pd.IsTx {
		data.reqs = append(data.reqs, *pd)
		return
	}

	var resp *PacketData
	var respFup *PacketData
	req := getPkt(data.reqs, pd.GetSequenceID())
	if req == nil {
		LogWarn("No matching Req for Resp")
		return
	}
	if pd.IsPDelayResp() {
		resp = pd
		data.resps = append(data.resps, *resp)
		if resp.IsTwostepFlagSet() {
			respFup = getPkt(data.respFups, resp.GetSequenceID())
			if respFup == nil {
				return
			}
		}
	} else if pd.IsPDelayRespFollowUp() {
		respFup = pd
		data.respFups = append(data.respFups, *respFup)
		resp = getPkt(data.resps, respFup.GetSequenceID())
		if resp == nil {
			return
		}
	} else if pd.IsDelayResp() {
		resp = pd
	}
	// Calculate
	if *count >= data.count {
		*running = false
	}
	if data.do.Peertopeer {
		ns := CalcPDelay(req, resp, respFup)
		if ns == nil {
			LogWarn("Failed to calculate pdelay\n")
			return
		}
		if data.app.Cli {
			fmt.Fprintf(data.app.W, "PDelay seq %d: %d\n", req.GetSequenceID(), *ns)
		} else {
			LogDebug("PDelay seq %d: %d\n", req.GetSequenceID(), *ns)
		}
	} else {
		if !data.app.Cli {
			return
		}
		t4 := resp.GetDelayRespOriginTimestamp().Time()
		t4_ns := t4.Nanosecond()
		t4_s := t4.Unix()
		t3 := req.HwTstamp
		t3_ns := t3.Nanosecond()
		t3_s := t3.Unix()
		cf := resp.GetCorrectionField()
		str := fmt.Sprintf("Seq %d | ReqTS %d.%09d | RespTs %d.%09d | Cf %d\n",
			req.GetSequenceID(), t3_s, t3_ns, t4_s, t4_ns, cf)
		if data.app.Cli {
			fmt.Fprint(data.app.W, str)
		} else {
			LogDebug("%s", str)
		}
	}
}

// TODO: Port needs delayed exit to process the final requests sent.
func client(app *App, do *DelayOpts, port *Port) {
	if !app.Cli {
		LogNotice("Starting Delay Mode: client on %s", port.Name)
	} else {
		fmt.Fprintf(app.W, "Transmitting...\n")
	}
	app.EnableSignals()
	var wg sync.WaitGroup
	wg.Add(1)
	// Use Sequence system to define what we send. Allows using Count correctly.
	cnt := do.Count
	cfg := Config{TxSequenceInterval: do.IntervalTime, Count: &cnt}
	if do.Peertopeer {
		cfg.SequenceTypes = append(cfg.SequenceTypes, ptp.MessagePDelayReq)
	} else {
		cfg.SequenceTypes = append(cfg.SequenceTypes, ptp.MessageDelayReq)
	}
	pkts := make(chan PacketData, 100)
	data := clientData{
		reqs:     make([]PacketData, 0, 100),
		resps:    make([]PacketData, 0, 100),
		respFups: make([]PacketData, 0, 100),
		app:      app,
		do:       do,
		count:    do.Count,
	}
	go port.RunPort(app, cfg, &wg, pkts)
	pktCount := uint32(0)

	running := true
	for running {
		select {
		case <-app.Sigs:
			running = false
		case <-app.QuitCh:
			running = false
		case pd, ok := <-pkts:
			if !ok {
				running = false
				continue
			}
			process_packet(&data, &pd, &pktCount, &running)
		}
	}
	port.Exit()
	wg.Wait()
	app.DisableSignals()
	if !app.Cli {
		LogNotice("Exiting Delay Mode: client on %s", port.Name)
	}
}

func getPkt(pkts []PacketData, seq uint16) *PacketData {
	// Reverse search since the one we want is likely the last
	length := len(pkts)
	for i := range length {
		pd := pkts[length-i-1]
		if pd.GetSequenceID() == seq {
			return &pd
		}
	}
	return nil
}

func server(app *App, do *DelayOpts, port *Port) {
	if !app.Cli {
		LogNotice("Starting Delay Mode: server on %s", port.Name)
	}
	app.EnableSignals()
	var wg sync.WaitGroup
	wg.Add(1)
	cfg := Config{}
	// For now, reply to requests regardless of mode
	// if do.Peertopeer {
	cfg.ReplyPDelay = true
	// } else {
	cfg.ReplyDelay = true
	// }
	pkts := make(chan PacketData, 100)
	go port.RunPort(app, cfg, &wg, pkts)

	running := true
	for running {
		select {
		case <-app.Sigs:
			running = false
		case <-app.QuitCh:
			running = false
		case pd := <-pkts:
			fmt.Fprintf(app.W, "%v", pd.ToString())
		}
	}
	port.Exit()
	wg.Wait()
	app.DisableSignals()
	if !app.Cli {
		LogNotice("Exiting Delay Mode: server on %s", port.Name)
	}
}
