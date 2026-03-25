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

func PktMode() {
	var port Port
	var opts = Options{}
	domain := 0
	interval := 1000
	help := false

	getopt.FlagLong(&domain, "domain", 'd', "PTP domain")
	getopt.FlagLong(&interval, "interval", 'I', "TX packet interval (ms)")
	getopt.FlagLong(&opts.Iface, "iface", 'i', "Interface to operate on")
	getopt.FlagLong(&opts.Seq, "seq", 's', "Starting SequenceID")
	getopt.FlagLong(&opts.IngressLatency, "ilat", 0, "Ingress latency")
	getopt.FlagLong(&opts.EgressLatency, "elat", 0, "Egress latency")
	getopt.FlagLong(&opts.RxMode, "rx", 'r', "Receive only")
	getopt.FlagLong(&opts.Udp, "", '4', "Use UDP instead of L2")
	getopt.FlagLong(&opts.Count, "count", 'c', "Number of packets to transmit")
	getopt.FlagLong(&help, "help", 'h', "Show this help menu")
	getopt.Parse()
	if help {
		getopt.Usage()
		return
	}

	if opts.Iface == "" {
		fmt.Println("Must specify interface with -if")
		return
	}
	if domain > 255 {
		fmt.Println("Domain must be 0-255")
		return
	}
	if interval < 0 {
		fmt.Println("Interval must be >= 0")
		return
	}
	opts.Domain = uint8(domain)
	opts.Interval = time.Duration(interval) * time.Millisecond

	err := port.Init(opts, 0, 1)
	if err != nil {
		log.Fatalf("Failed initializing port: %s", err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	if opts.RxMode {

		ch := make(chan PacketData, 100)
		quit := make(chan int)

		go port.RxMode(ch, quit)

		running := true
		for running {
			select {
			case <-sigs:
				quit <- 0
				running = false
			case pd := <-ch:
				pd.Print()
			}
		}
	} else {
		rxCh := make(chan PacketData, 100)
		quit := make(chan int)

		txCh := make(chan PacketData, 100)
		outCh := make(chan PacketData, 100)
		// quit := make(chan int)
		timer := time.NewTicker(1 * time.Second)

		go port.TxMode(txCh, outCh, quit)
		go port.RxMode(rxCh, quit)

		count := 0
		seq := opts.Seq
		running := true
		for running {
			select {
			case <-sigs:
				quit <- 0
				running = false
			case pd2 := <-rxCh:
				pd2.Print()
			// case <-sigs:
			// running = false
			case pd2, ok := <-outCh:
				if !ok {
					running = false
					continue
				}
				if pd2.Packet == nil {
					continue
				}
				pd2.Print()
			case <-timer.C:
				pkt, err := port.BuildPacket(ptp.MessageSync, seq)
				if err != nil {
					log.Fatalf("Failed building packet: %s", err)
				}
				pd := PacketData{Packet: pkt, IsTx: true}
				txCh <- pd
				seq += 1
				count += 1
				if count >= opts.Count {
					close(txCh)
					timer.Stop()
					continue
				}
			}
		}
	}
	// TODO: Requires HW to test
	timestamp.DisableTimestamps(port.EFd, port.Interface)
}
