// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	. "ckeep/internal"
	"fmt"
	"log"
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

func PktMode() {
	mode := "pkt"
	var opts = CommonOpts{Mode: mode}
	var pktOpts = PktOpts{}
	pktOpts.Count = 1
	pktOpts.Interval = uint32(1000)
	pktOpts.Vlan = -1
	pktOpts.Prio = -1
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

	// TODO: Allow setting interval 0 to skip using ticker
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
		fmt.Printf("No port selected\n")
		return
	}

	err := opts.ValidateTstampMode(pname)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	port := Port{
		IfaceStr: pname,
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
		log.Fatalf("Failed initializing port: %s", err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	rxCh := make(chan PacketData, 100)
	quit := make(chan int)
	var ticker *time.Ticker
	txCount := uint32(1)
	infinite := false
	running := true

	if noWait {
		noWaitMode(&port, &pktOpts, sigs)
		return
	}

	go port.RxMode(rxCh, quit)
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
		txPackets(&port, &pktOpts)
	}

	if !app.Cli {
		log.Printf("Starting Packet Mode")
	}
	for running {
		select {
		case <-sigs:
			running = false
		case _ = <-rxCh:
		case <-ticker.C:
			txPackets(&port, &pktOpts)
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
	close(quit)
	port.Deinit()
	if !app.Cli {
		log.Printf("Exiting Packet Mode")
	}
}

func noWaitMode(port *Port, pktOpts *PktOpts, sigs chan os.Signal) {
	txCount := uint32(1)
	running := true
	infinite := (pktOpts.Count == 0)
	txPackets(port, pktOpts)
	for running {
		select {
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

}

func txSinglePacket(port *Port, pktOpts *PktOpts, msgtype ptp.MessageType) {
	pd, err := port.BuildPacket(msgtype, pktOpts.Seq)
	if err != nil {
		log.Fatalf("Failed building packet: %s", err)
	}
	port.Transmit(pd)
	pd.Print()
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
