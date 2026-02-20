package main

import (
	//"bytes"
	"encoding/binary"
	"os"

	//"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	//"os"
	ptp "github.com/facebook/time/ptp/protocol"
	timestamp "github.com/facebook/time/timestamp"
)

type Layer int

const (
	LayerMac Layer = iota
	LayerUDPv4
	LayerUDPv6
)

const (
	event_port = 319
)

type Port struct {
	IfaceStr  string
	IP        net.IP
	DestIP    net.IP
	Interface *net.Interface
	Layer     Layer
	EFd       int
}

type PacketData struct {
	packet   ptp.Packet
	rxtstamp time.Time
}

type Options struct {
	transportSpecific uint8
	twoStepFlag_set   bool
	human_readable    bool
	ingressLatency    uint64
	egressLatency     uint64
	header_offset     uint
	nonstop_flag      bool
	twoStepFlag       bool
	interval          time.Duration
	pkttype           *ptp.MessageType
	minor_version     uint8
	domain            uint8
	count             int
	vlan              *int
	prio              int
	seq               uint16

	rx_mode    bool
	iface      string
	delay_mode string
	clk_type   string
	udp        bool

	// tstamp_all        int
	// auto_fup          int
	// listen   int
	// unsigned char mac[ETH_ALEN];
	// char *interface;
	// int sequence_types[SEQUENCE_MAX];
	// int sequence_length;
	// enum delay_mechanism dm;
	// enum hwtstamp_clk_types clk_type;

}

func htons(i uint16) uint16 {
	return (i<<8)&0xff00 | i>>8
}

func (p *Port) openSocket() error {
	if p.Layer == LayerMac {
		eFd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, syscall.ETH_P_ALL)
		if err != nil {
			return err
		}
		interf, err := net.InterfaceByName(p.IfaceStr)
		if err != nil {
			return err
		}
		p.Interface = interf
		p.EFd = eFd

		sll := &syscall.SockaddrLinklayer{
			Protocol: htons(syscall.ETH_P_ALL),
			Ifindex:  interf.Index,
		}
		if err := syscall.Bind(eFd, sll); err != nil {
			log.Fatalf("bind to %s failed: %v", p.IfaceStr, err)
		}

		// fmt.Printf("Opened L2 socket on %s\n", p.IfaceStr)
	} else if p.Layer == LayerUDPv4 {
		eventConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: p.IP, Port: event_port})
		if err != nil {
			log.Fatalf("Listening error: %s", err)
		}
		// defer eventConn.Close()
		// get connection file descriptor
		eFd, err := timestamp.ConnFd(eventConn)
		p.EFd = eFd
	} else {
		log.Fatal("openSocket: not implemented")
	}
	return nil
}

func (port *Port) transmit(syncP *ptp.SyncDelayReq) error {
	buf := make([]byte, timestamp.PayloadSizeBytes)

	n, err := ptp.BytesTo(syncP, buf)
	n = n - 2 // Trim the unused TLV
	if err != nil {
		log.Fatalf("Failed to generate the sync packet: %v", err)
	}

	if port.Layer == LayerMac {
		hdr := []byte{0x01, 0x1b, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xaa, 0xaa, 0xaa, 0x88, 0xf7}
		packet := append(hdr, buf[:n]...)
		// fmt.Println(len(packet), n)
		var addr syscall.SockaddrLinklayer
		addr.Protocol = syscall.ETH_P_1588
		addr.Ifindex = port.Interface.Index
		addr.Hatype = syscall.ARPHRD_ETHER

		err = syscall.Sendto(port.EFd, packet, 0, &addr)
	} else if port.Layer == LayerUDPv4 {
		eclisa := timestamp.IPToSockaddr(port.DestIP, event_port)
		err = unix.Sendto(port.EFd, buf[:n], 0, eclisa)
	} else {
		log.Fatal("transmit: not implemented")
	}
	return err
}

func (port *Port) receive(buf []byte, oob []byte) (*PacketData, error) {
	switch port.Layer {
	case LayerMac:
		bytes, _, rxTS, err := timestamp.ReadPacketWithRXTimestampBuf(port.EFd, buf, oob)
		p, err := ptp.DecodePacket(buf[14:bytes])
		if err != nil {
			return nil, err
		}
		data := &PacketData{
			packet:   p,
			rxtstamp: rxTS,
		}
		return data, nil
	case LayerUDPv4:
		_, _, rxTS, err := timestamp.ReadPacketWithRXTimestampBuf(port.EFd, buf, oob)
		// TODO: Parse packet
		if err != nil {
			return nil, err
		}
		fmt.Println("RX TS:", rxTS.UnixNano())
	default:
		log.Fatal("receive: not implemented")
	}
	return nil, nil
}

func (port *Port) rx_mode(opts *Options, c chan PacketData) error {

	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	for {
		pd, err := port.receive(buf, oob)
		if err != nil {
			continue
		}
		c <- *pd
	}
}

func (port *Port) tx_mode(opts *Options) {
	syncP := &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})), //#nosec G115
			DomainNumber:    opts.domain,
			FlagField:       ptp.FlagTwoStep,
			SequenceID:      opts.seq,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: 0x000000fffeaa0000,
			},
			LogMessageInterval: 0,
			ControlField:       0,
		},
	}

	oob := make([]byte, timestamp.ControlSizeBytes)
	toob := make([]byte, timestamp.ControlSizeBytes)
	for _ = range opts.count {
		err := port.transmit(syncP)
		syncP.Header.SequenceID += 1
		if err != nil {
			continue
		}
		txTS, _, err := timestamp.ReadTXtimestampBuf(port.EFd, oob, toob)
		if err != nil {
			continue
		}
		fmt.Println(txTS.UnixNano())
		time.Sleep(opts.interval)
	}
}

func pkt_mode(args []string) {
	var port Port
	var opts = Options{}

	fs := flag.NewFlagSet("pkt", flag.ContinueOnError)
	fs.StringVar(&opts.iface, "if", "", "Interface name to operate on")
	fs.BoolVar(&opts.rx_mode, "r", false, "Receive mode")
	fs.BoolVar(&opts.udp, "4", false, "Use UDP instead of L2")
	fs.IntVar(&opts.count, "c", 1, "Number of packets to transmit")
	fs.Uint64Var(&opts.ingressLatency, "ilat", 0, "Ingress latency")
	fs.Uint64Var(&opts.egressLatency, "elat", 0, "Egress latency")
	var interval = fs.Uint("interval", 1000, "TX packet interval (ms)")
	var domain = fs.Int("domain", 0, "PTP domain")
	var seq = fs.Int("seq", 0, "Starting SequenceID")

	err := fs.Parse(args)

	if err != nil {
		return
	}

	if opts.iface == "" {
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
	opts.domain = uint8(*domain)
	opts.interval = time.Duration(*interval) * time.Millisecond
	opts.seq = uint16(*seq)
	// fmt.Printf("%v\n", opts)
	// fmt.Printf("Port %s\n", opts.iface)

	if opts.udp {
		var ip net.IP
		var dest net.IP
		if !opts.rx_mode {
			ip = net.IPv4(10, 11, 0, 1)
			// dest := net.IPv4(224, 0, 1, 129)
			dest = net.IPv4(10, 11, 0, 2)
		} else {
			ip = net.IPv4(10, 11, 0, 2)
			dest = net.IPv4(10, 11, 0, 1)
		}
		port.IfaceStr = opts.iface
		port.Layer = LayerUDPv4
		port.IP = ip
		port.DestIP = dest
	} else {
		port.IfaceStr = opts.iface
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

	err = port.openSocket()

	if err != nil {
		log.Fatalf("Getting event connection FD: %s", err)
	}

	// Enable RX timestamps. Delay requests need to be timestamped by ptp4u on receipt
	netif, err := net.InterfaceByName(port.IfaceStr)
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

	if opts.rx_mode {
		c := make(chan PacketData)

		go port.rx_mode(&opts, c)

		for pd := range c {
			msgtype := pd.packet.MessageType()
			sync, ok := pd.packet.(*ptp.SyncDelayReq)
			if !ok {
				fmt.Println("Received packet was not a Sync")
			}
			rx_ns := pd.rxtstamp.UnixNano() % 1000000000
			rx_s := pd.rxtstamp.Unix()
			seq := sync.Header.SequenceID
			corr := sync.Header.CorrectionField.Duration()
			domain := sync.Header.DomainNumber
			fmt.Printf("%s | Seq %d | Dom %d | RXts %d.%09d | Corr %d\n", msgtype, seq, domain, rx_s, rx_ns, corr)
		}
	} else {
		port.tx_mode(&opts)
	}
}

func main() {
	mode := os.Args[1]
	args := os.Args[2:]
	if mode == "pkt" {
		pkt_mode(args)
		return
	}
}
