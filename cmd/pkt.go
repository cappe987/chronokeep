package cmd

import (
	"flag"
	"fmt"
	. "intime/internal"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	timestamp "github.com/facebook/time/timestamp"
	"golang.org/x/sys/unix"
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

	if opts.Udp {
		var ip net.IP
		var dest net.IP
		if !opts.RxMode {
			ip = net.IPv4(10, 11, 0, 1)
			// dest := net.IPv4(224, 0, 1, 129)
			dest = net.IPv4(10, 11, 0, 2)
		} else {
			ip = net.IPv4(10, 11, 0, 2)
			dest = net.IPv4(10, 11, 0, 1)
		}
		port.IfaceStr = opts.Iface
		port.Layer = LayerUDPv4
		port.IP = ip
		port.DestIP = dest
		port.RecordPackets = true
	} else {
		port.IfaceStr = opts.Iface
		port.Layer = LayerMac
		port.RecordPackets = true
	}
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

	port.Init(opts)
	err = port.OpenSocket()

	if err != nil {
		log.Fatalf("Getting event connection FD: %s", err)
	}

	// Enable RX timestamps. Delay requests need to be timestamped by ptp4u on receipt
	netif, err := net.InterfaceByName(port.IfaceStr)
	port.Interface = netif
	if err != nil {
		log.Fatalf("Failed fetching interface")
	}
	if err := timestamp.EnableTimestamps(timestamp.SW, port.EFd, netif); err != nil {
		log.Fatal(err)
	}

	err = unix.SetNonblock(port.EFd, false)
	if err != nil {
		log.Fatalf("Failed to set socket to blocking: %s", err)
	}

	if opts.RxMode {
		tmo := unix.Timeval{
			Sec:  0,
			Usec: 100000, // 100 ms
		}
		unix.SetsockoptTimeval(port.EFd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tmo)

		ch := make(chan PacketData, 100)
		quit := make(chan int)
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

		go port.RxMode(ch, quit)

		running := true
		for running {
			select {
			case <-sigs:
				quit <- 0
				running = false
			case pd := <-ch:
				pd.Print("rxts")
			}
		}
		// TODO: Requires HW to test
		timestamp.DisableTimestamps(port.EFd, port.Interface)
	} else {
		// port.TxMode()
		txCh := make(chan PacketData, 100)
		outCh := make(chan PacketData, 100)
		quit := make(chan int)
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		timer := time.NewTicker(1 * time.Second)

		go port.TxMode(txCh, outCh, quit)

		count := 0
		seq := opts.Seq
		running := true
		for running {
			select {
			case <-sigs:
				// quit <- 0
				running = false
			case pd2, ok := <-outCh:
				if !ok {
					running = false
					continue
				}
				if pd2.Packet == nil {
					continue
				}
				pd2.Print("txts")
			case <-timer.C:
				pkt, err := port.BuildPacket(ptp.MessageSync, seq)
				if err != nil {
					log.Fatalf("Failed building packet: %s", err)
				}
				pd := PacketData{Packet: pkt}
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
}
