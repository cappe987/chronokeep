package cmd

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	. "ptpan/internal"
	"syscall"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	timestamp "github.com/facebook/time/timestamp"
	"golang.org/x/sys/unix"
)

func HandleRxPacket(pd PacketData) {
	msgtype := pd.Packet.MessageType()
	sync, ok := pd.Packet.(*ptp.SyncDelayReq)
	if !ok {
		fmt.Println("Received packet was not a Sync")
	}
	rx_ns := pd.Rxtstamp.UnixNano() % 1000000000
	rx_s := pd.Rxtstamp.Unix()
	seq := sync.Header.SequenceID
	corr := sync.Header.CorrectionField.Duration()
	domain := sync.Header.DomainNumber
	fmt.Printf("%s | Seq %d | Dom %d | RXts %d.%09d | Corr %d\n", msgtype, seq, domain, rx_s, rx_ns, corr)
}

func PktMode(args []string) {
	var port Port
	var opts = Options{}

	// TODO: Replace with Cobra + Viper
	fs := flag.NewFlagSet("pkt", flag.ContinueOnError)
	fs.StringVar(&opts.Iface, "if", "", "Interface name to operate on")
	fs.BoolVar(&opts.Rx_mode, "r", false, "Receive mode")
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
		if !opts.Rx_mode {
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
	} else {
		port.IfaceStr = opts.Iface
		port.Layer = LayerMac
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

	if opts.Rx_mode {
		tmo := unix.Timeval{
			Sec:  0,
			Usec: 100000, // 100 ms
		}
		unix.SetsockoptTimeval(port.EFd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tmo)

		ch := make(chan PacketData, 100)
		quit := make(chan int)
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

		go port.RxMode(&opts, ch, quit)

		running := true
		for running {
			select {
			case <-sigs:
				quit <- 0
				running = false
			case pd := <-ch:
				HandleRxPacket(pd)
			}
		}
		// TODO: Requires HW to test
		timestamp.DisableTimestamps(port.EFd, port.Interface)
	} else {
		port.TxMode(&opts)
	}
}
