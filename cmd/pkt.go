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
	TwoStepFlag_set bool
	Human_readable  bool
	TwoStepFlag     bool
	Interval        time.Duration
	Pkttype         *ptp.MessageType
	Count           uint32
	Seq             uint16

	RxMode     bool
	Delay_mode string
	Clk_type   string

	// tstamp_all        int
	// auto_fup          int
	// listen   int
	// int sequence_types[SEQUENCE_MAX];
	// int sequence_length;

	// TODO: time/timestamp needs to be patched to support this.
	// enum delay_mechanism dm;
	// enum hwtstamp_clk_types clk_type;
	// header_offset     uint
}

func PktMode() {
	var port Port
	var opts = CommonOpts{}
	var pktOpts = PktOpts{}
	interval := uint32(1000)
	pktOpts.Count = 1
	opts.DestIp = "224.0.1.129"    // TODO: 224.0.0.107 for pdelays
	opts.Mac = "01:1b:19:00:00:00" // TODO: 01:80:c2:00:00:0e for pdelays

	opts.DefineCommonFlags()
	getopt.FlagLong(&interval, "interval", 'I', "TX packet interval (ms)")
	getopt.FlagLong(&pktOpts.Seq, "seq", 's', "Starting SequenceID")
	getopt.FlagLong(&pktOpts.RxMode, "rx", 'r', "Receive only")
	getopt.FlagLong(&pktOpts.Count, "count", 'c', "Number of packets to transmit. 0=infinite")
	getopt.Parse()
	opts.Validate()
	if interval < 0 {
		fmt.Println("Interval must be >= 0")
		return
	}
	// TODO: Allow setting interval 0 to skip using ticker
	pktOpts.Interval = time.Duration(interval) * time.Millisecond

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
			ticker = time.NewTicker(pktOpts.Interval)
			if pktOpts.Count == 0 {
				infinite = true
			}
		}
		txPacket(&port, &pktOpts)
	}

	for running {
		select {
		case <-sigs:
			quit <- 0
			running = false
		case pd := <-rxCh:
			pd.Print()
		case <-ticker.C:
			txPacket(&port, &pktOpts)
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

func txPacket(port *Port, pktOpts *PktOpts) {
	pd, err := port.BuildPacket(ptp.MessageSync, pktOpts.Seq)
	if err != nil {
		log.Fatalf("Failed building packet: %s", err)
	}
	port.Transmit(pd)
	pd.Print()
	// if pd.IsSync() {
	// 	fup, err := port.MakeFollowUp(pd)
	// 	if err == nil {
	// 		port.Transmit(fup)
	// 		fup.Print()
	// 	}
	// }
	pktOpts.Seq += 1
}
