package cmd

import (
	"flag"
	"fmt"
	. "intime/internal"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	timestamp "github.com/facebook/time/timestamp"
)

func PktMode(args []string) {
	var port Port
	var opts = Options{}

	// TODO: Replace with getopt?
	fs := flag.NewFlagSet("pkt", flag.ContinueOnError)
	fs.StringVar(&opts.Iface, "if", "", "Interface name to operate on")
	fs.BoolVar(&opts.RxMode, "r", false, "Receive mode")
	fs.BoolVar(&opts.Udp, "4", false, "Use UDP instead of L2")
	fs.IntVar(&opts.Count, "c", 1, "Number of packets to transmit")
	fs.Uint64Var(&opts.IngressLatency, "ilat", 0, "Ingress latency")
	fs.Uint64Var(&opts.EgressLatency, "elat", 0, "Egress latency")
	var interval = fs.Uint("interval", 1000, "TX packet interval (ms)")
	var domain = fs.Int("domain", 0, "PTP domain")
	var seq = fs.Int("seq", 0, "Starting SequenceID")

	err := fs.Parse(args)

	if err != nil {
		return
	}

	if opts.Iface == "" {
		fmt.Println("Must specify interface with -if")
		return
	}
	if *domain > 255 {
		fmt.Println("Domain must be 0-255")
		return
	}
	if *interval < 0 {
		fmt.Println("Interval must be >= 0")
		return
	}
	opts.Domain = uint8(*domain)
	opts.Interval = time.Duration(*interval) * time.Millisecond
	opts.Seq = uint16(*seq)

	//fmt.Println("Hello, World!")
	// if use_l2 {
	// 	eFd, err = syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, syscall.ETH_P_ALL)
	// 	interf, err = net.InterfaceByName(iface)
	// } else {
	// 	eventConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: port})
	// 	if err != nil {
	// 		log.Fatalf("Listening error: %s", err)
	// 	}
	// 	defer eventConn.Close()
	// 	// get connection file descriptor
	// 	eFd, err = timestamp.ConnFd(eventConn)
	// }

	err = port.Init(opts)
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
		ch := make(chan PacketData, 100)
		quit := make(chan int)

		txCh := make(chan PacketData, 100)
		outCh := make(chan PacketData, 100)
		// quit := make(chan int)
		timer := time.NewTicker(1 * time.Second)

		go port.TxMode(txCh, outCh, quit)
		go port.RxMode(ch, quit)

		count := 0
		seq := opts.Seq
		running := true
		for running {
			select {
			case <-sigs:
				quit <- 0
				running = false
			case pd2 := <-ch:
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
