package cmd

import (
	"fmt"
	. "intime/internal"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	timestamp "github.com/facebook/time/timestamp"
	"github.com/pborman/getopt/v2"
)

type PktOpts struct {
	// Configuration file
	Interval uint32
	Count    uint32
	Seq      uint16
	Sequence []string
	RxMode   bool
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
	var port Port
	var opts = CommonOpts{Mode: "pkt"}
	var pktOpts = PktOpts{}
	pktOpts.Count = 1
	pktOpts.Interval = uint32(1000)
	noWait := false

	opts.DefineCommonFlags()
	opts.AddModeOpt("pkt", &pktOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt("pkt", &pktOpts.Seq, 's', "seq", "<seqid>", "Starting SequenceID")
	opts.AddModeOpt("pkt", &pktOpts.RxMode, 'r', "rx", "", "Receive only")
	opts.AddModeOpt("pkt", &pktOpts.Count, 'c', "count", "<num>", "Number of packets to transmit. 0=infinite")

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

	err := port.Init(opts, 0, 1)
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

	for running {
		select {
		case <-sigs:
			quit <- 0
			running = false
		case pd := <-rxCh:
			pd.Print()
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
	// TODO: Requires HW to test
	timestamp.DisableTimestamps(port.EFd, port.Interface)
}

func noWaitMode(port *Port, pktOpts *PktOpts, sigs chan os.Signal) {
	fmt.Printf("hello\n")
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
