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
	opts.Count = 1
	opts.DestIp = "224.0.1.129"
	opts.Mac = "01:19:1b:00:00:00" // TODO: 01:80:c2:00:00:0e for pdelays

	getopt.FlagLong(&domain, "domain", 'd', "PTP domain")
	getopt.FlagLong(&interval, "interval", 'I', "TX packet interval (ms)")
	getopt.FlagLong(&opts.Iface, "iface", 'i', "Interface to operate on")
	getopt.FlagLong(&opts.Seq, "seq", 's', "Starting SequenceID")
	getopt.FlagLong(&opts.IngressLatency, "ilat", 0, "Ingress latency")
	getopt.FlagLong(&opts.EgressLatency, "elat", 0, "Egress latency")
	getopt.FlagLong(&opts.RxMode, "rx", 'r', "Receive only")
	getopt.FlagLong(&opts.Udp, "", '4', "Use UDP instead of L2")
	getopt.FlagLong(&opts.Count, "count", 'c', "Number of packets to transmit. 0=infinite")
	getopt.FlagLong(&opts.Ip, "sip", 0, "Source IP for UDP mode")
	getopt.FlagLong(&opts.DestIp, "dip", 0, "Destination IP for UDP mode")
	getopt.FlagLong(&opts.Mac, "mac", 'm', "Destination MAC for L2 mode")
	getopt.FlagLong(&help, "help", 'h', "Show this help menu")
	getopt.Parse()
	if help {
		getopt.Usage()
		return
	}

	if opts.Udp {
		if opts.Ip == "" {
			fmt.Println("Must specify source IP with --sip")
			return
		} else if opts.Iface == "" {
			iface, err := InterfaceFromIP(opts.Ip)
			if err != nil {
				fmt.Printf("Unable to find interface with IP %s\n", opts.Ip)
				return
			}
			opts.Iface = iface
		}
	} else {
		if opts.Iface == "" {
			fmt.Println("Must specify interface with --iface")
			return

		}
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
	rxCh := make(chan PacketData, 100)
	quit := make(chan int)
	if opts.RxMode {
		go port.RxMode(rxCh, quit)

		running := true
		for running {
			select {
			case <-sigs:
				quit <- 0
				running = false
			case pd := <-rxCh:
				pd.Print()
			}
		}
	} else {
		txCh := make(chan PacketData, 100)
		outCh := make(chan PacketData, 100)
		ticker := time.NewTicker(opts.Interval)

		go port.TxMode(txCh, outCh, quit)
		go port.RxMode(rxCh, quit)

		count := 1
		infinite := false
		txPacket(port, &opts, txCh)
		if opts.Count == 0 {
			infinite = true
		} else if opts.Count == 1 {
			stopTx(txCh, ticker)
		}
		running := true
		for running {
			select {
			case <-sigs:
				quit <- 0
				running = false
			case pd := <-rxCh:
				pd.Print()
			case pd, ok := <-outCh:
				if !ok {
					running = false
					continue
				}
				if pd.Packet == nil {
					fmt.Printf("Invalid packet from txtstamp\n")
					continue
				}
				pd.Print()
			case <-ticker.C:
				txPacket(port, &opts, txCh)
				if infinite {
					continue
				}
				count += 1
				if count >= opts.Count {
					stopTx(txCh, ticker)
					continue
				}
			}
		}
	}
	// TODO: Requires HW to test
	timestamp.DisableTimestamps(port.EFd, port.Interface)
}

func stopTx(txCh chan PacketData, ticker *time.Ticker) {
	close(txCh)
	ticker.Stop()
}

func txPacket(port Port, opts *Options, txCh chan PacketData) {
	pkt, err := port.BuildPacket(ptp.MessageSync, opts.Seq)
	if err != nil {
		log.Fatalf("Failed building packet: %s", err)
	}
	pd := PacketData{Packet: pkt, IsTx: true}
	txCh <- pd
	opts.Seq += 1
}
