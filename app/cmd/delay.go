// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	"fmt"
	. "intime/internal"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type DelayOpts struct {
	// Configuration file
	Ports    map[string]PortOpts
	Interval uint32
	// Count    uint32
	Seq        uint16
	Vlan       int
	Prio       int
	Client     bool
	Server     bool // Server replies to requests sent
	Peertopeer bool

	// Internal fields
	IntervalTime time.Duration
}

func DelayMode() {
	mode := "delay"
	var opts = CommonOpts{Mode: mode}
	var delayOpts = DelayOpts{}
	delayOpts.Interval = uint32(1000)
	delayOpts.Vlan = -1
	delayOpts.Prio = -1
	opts.Iface = "dummy"
	opts.Ip = "dummy"

	opts.DefineCommonFlags()
	opts.AddModeOpt(mode, &delayOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt(mode, &delayOpts.Client, 'c', "client", "", "Run client")
	opts.AddModeOpt(mode, &delayOpts.Server, 's', "server", "", "Run server")
	opts.AddModeOpt(mode, &delayOpts.Vlan, 'V', "vlan", "<VID>", "VLAN ID")
	opts.AddModeOpt(mode, &delayOpts.Prio, 'p', "prio", "<PRIO>", "VLAN Prio")
	opts.AddModeOpt(mode, &delayOpts.Peertopeer, 'P', "peertopeer", "", "P2P mode")

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
		fmt.Printf("No port selected\n")
		return
	}

	port := Port{
		IfaceStr: pname,
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

	err := port.Init(app, 1)
	if err != nil {
		log.Fatalf("Failed initializing port: %s", err)
	}

	if delayOpts.Client {
		client(app, &delayOpts, &port)
	} else if delayOpts.Server {
		server(app, &delayOpts, &port)
	} else {
		fmt.Printf("Please select -c/--client or -s/--server\n")
	}
}

func client(app *App, do *DelayOpts, port *Port) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	rxCh := make(chan PacketData, 100)
	quit := make(chan int)
	var ticker *time.Ticker
	running := true
	reqs := make([]PacketData, 0, 100)
	resps := make([]PacketData, 0, 100)
	respFups := make([]PacketData, 0, 100)

	go port.RxMode(rxCh, quit)
	ticker = time.NewTicker(do.IntervalTime)

	if app.Cli {
		fmt.Printf("Transmitting...\n")
	}
	for running {
		select {
		case <-sigs:
			quit <- 0
			running = false
		case pd := <-rxCh:
			var resp *PacketData
			var respFup *PacketData
			req := getPkt(reqs, pd.GetSequenceID())
			if req == nil {
				fmt.Printf("No matching Req for Resp")
				continue
			}
			if pd.IsPDelayResp() {
				resp = &pd
				resps = append(resps, *resp)
				if resp.IsTwostepFlagSet() {
					respFup = getPkt(respFups, resp.GetSequenceID())
					if respFup == nil {
						continue
					}
				}
			} else if pd.IsPDelayRespFollowUp() {
				respFup = &pd
				respFups = append(respFups, *respFup)
				resp = getPkt(resps, respFup.GetSequenceID())
				if resp == nil {
					continue
				}
			} else if pd.IsDelayResp() {
				resp = &pd
			}
			// Calculate
			if do.Peertopeer {
				ns := CalcPDelay(req, resp, respFup)
				if ns == nil {
					fmt.Printf("Failed to calculate pdelay\n")
					continue
				}
				if !app.Cli {
					continue
				}
				fmt.Printf("PDelay seq %d: %d\n", req.GetSequenceID(), *ns)

			} else {
				if !app.Cli {
					continue
				}
				t4 := resp.GetDelayRespOriginTimestamp().Nano()
				t4_ns := t4 % 1000000000
				t4_s := t4 / 1000000000
				t3 := req.HwTstamp.UnixNano()
				t3_ns := t3 % 1000000000
				t3_s := t3 / 1000000000
				cf := resp.GetCorrectionField()
				fmt.Printf("Seq %d | ReqTS %d.%09d | RespTs %d.%09d | Cf %d\n",
					req.GetSequenceID(), t3_s, t3_ns, t4_s, t4_ns, cf)
			}
		case <-ticker.C:
			var req *PacketData
			if do.Peertopeer {
				req = port.TransmitPDelayReq()
			} else {
				req = port.TransmitDelayReq()
			}
			reqs = append(reqs, *req)
		}
	}
	port.Deinit()
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
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	rxCh := make(chan PacketData, 100)
	quit := make(chan int)
	running := true

	if app.Cli {
		fmt.Printf("Listening...\n")
	}
	go port.RxMode(rxCh, quit)
	for running {
		select {
		case <-sigs:
			quit <- 0
			running = false
		case pd := <-rxCh:
			if !do.Peertopeer && pd.IsDelayReq() {
				port.ReplyToDelayReq(&pd)
			}
			if do.Peertopeer && pd.IsPDelayReq() {
				port.ReplyToPDelayReq(&pd)
			}
		}
	}
	port.Deinit()
}
