// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	. "ckeep/internal"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
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
		shutdown := make(chan int)
		client(app, &delayOpts, &port, os.Stdout, shutdown)
	} else if delayOpts.Server {
		shutdown := make(chan int)
		server(app, &delayOpts, &port, os.Stdout, shutdown)
	} else {
		LogError("Please select -c/--client or -s/--server\n")
	}
	port.Deinit()
}

func client(app *App, do *DelayOpts, port *Port, w io.Writer, shutdown chan int) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	rxCh := make(chan PacketData, 100)
	var ticker *time.Ticker
	running := true
	reqs := make([]PacketData, 0, 100)
	resps := make([]PacketData, 0, 100)
	respFups := make([]PacketData, 0, 100)

	go port.RxMode(rxCh)
	ticker = time.NewTicker(do.IntervalTime)
	count := uint32(0)

	if app.Cli {
		fmt.Fprintf(w, "Transmitting...\n")
	} else {
		LogNotice("Starting Delay Mode: client on %s", port.IfaceStr)
	}
	for running {
		select {
		case <-shutdown:
			running = false
		case <-sigs:
			running = false
		case pd := <-rxCh:
			var resp *PacketData
			var respFup *PacketData
			req := getPkt(reqs, pd.GetSequenceID())
			if req == nil {
				LogWarn("No matching Req for Resp")
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
			if count >= do.Count {
				running = false
			}
			if do.Peertopeer {
				ns := CalcPDelay(req, resp, respFup)
				if ns == nil {
					LogWarn("Failed to calculate pdelay\n")
					continue
				}
				if app.Cli {
					fmt.Fprintf(w, "PDelay seq %d: %d\n", req.GetSequenceID(), *ns)
				} else {
					LogDebug("PDelay seq %d: %d\n", req.GetSequenceID(), *ns)
				}
			} else {
				if !app.Cli {
					continue
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
				if app.Cli {
					fmt.Fprint(w, str)
				} else {
					LogDebug("%s", str)
				}
			}
		case <-ticker.C:
			var req *PacketData
			var err error
			if count >= do.Count {
				ticker.Stop()
				continue
			}
			count += 1
			if do.Peertopeer {
				req, err = port.TransmitPDelayReq()
			} else {
				req, err = port.TransmitDelayReq()
			}
			if err != nil {
				LogError("%s", err)
			} else {
				reqs = append(reqs, *req)
			}
		}
	}
	if !app.Cli {
		LogNotice("Exiting Delay Mode: client on %s", port.IfaceStr)
	}
	port.Quit()
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

func server(app *App, do *DelayOpts, port *Port, w io.Writer, shutdown chan int) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	rxCh := make(chan PacketData, 100)
	running := true

	if app.Cli {
		fmt.Fprintf(w, "Listening...\n")
	} else {
		LogNotice("Starting Delay Mode: server on %s", port.IfaceStr)
	}
	go port.RxMode(rxCh)
	for running {
		select {
		case <-shutdown:
			running = false
		case <-sigs:
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
	if !app.Cli {
		LogNotice("Exiting Delay Mode: server on %s", port.IfaceStr)
	}
	port.Quit()
}
