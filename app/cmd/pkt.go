// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	. "ckeep/internal"
	"os"
	"os/signal"
	"syscall"
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

	err = port.Init(app, 1)
	if err != nil {
		LogError("Failed initializing port: %s", err)
		return
	}

	if noWait {
		noWaitMode(app, &port, &pktOpts)
	} else {
		normalMode(app, &port, &pktOpts)
	}
}

func normalMode(app *App, port *Port, pktOpts *PktOpts) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	rxCh := make(chan PacketData, 100)
	var ticker *time.Ticker
	txCount := uint32(1)
	infinite := false
	running := true

	port.StartRxMode(rxCh)
	if pktOpts.RxMode {
		// Create a ticker and stop it just so the channel exists.
		// It will never be used in RX mode.
		ticker = time.NewTicker(time.Second * 100)
		ticker.Stop()
	} else { // Start TX
		if pktOpts.Count == 1 {
			running = false
		} else {
			ticker = time.NewTicker(pktOpts.IntervalTime)
			if pktOpts.Count == 0 {
				infinite = true
			}
		}
		txPackets(port, pktOpts)
	}

	if !app.Cli {
		LogNotice("Starting Packet Mode on %s", port.Name)
	}
	for running {
		select {
		case <-app.QuitCh:
			running = false
		case <-sigs:
			running = false
		case _ = <-rxCh:
		case <-ticker.C:
			txPackets(port, pktOpts)
			if infinite {
				continue
			}
			txCount += 1
			if txCount >= pktOpts.Count {
				ticker.Stop()
				running = false
				continue
			}
		}
	}
	port.Quit()
	if !app.Cli {
		LogNotice("Exiting Packet Mode on %s", port.Name)
	}
}

func noWaitMode(app *App, port *Port, pktOpts *PktOpts) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	txCount := uint32(1)
	running := true
	infinite := (pktOpts.Count == 0)
	txPackets(port, pktOpts)
	for running {
		select {
		case <-app.QuitCh:
			running = false
		case <-sigs:
			running = false
		default:
			txPackets(port, pktOpts)
			if infinite {
				continue
			}
			txCount += 1
			if txCount >= pktOpts.Count {
				running = false
			}
		}
	}
	port.Quit()
}

func txSinglePacket(port *Port, pktOpts *PktOpts, msgtype ptp.MessageType) {
	pd, err := port.BuildPacket(msgtype, pktOpts.Seq)
	if err != nil {
		LogWarn("Failed building packet: %s", err)
		return
	}
	port.Transmit(pd)
	port.ShowPacket(pd)
}

func txPackets(port *Port, pktOpts *PktOpts) {
	if len(pktOpts.SequenceTypes) == 0 {
		txSinglePacket(port, pktOpts, ptp.MessageSync)
	} else {
		for _, msgtype := range pktOpts.SequenceTypes {
			txSinglePacket(port, pktOpts, msgtype)
		}
	}
	pktOpts.Seq += 1
}
