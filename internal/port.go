package internal

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"syscall"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	timestamp "github.com/facebook/time/timestamp"
	"golang.org/x/sys/unix"
)

type Layer int

const (
	LayerMac Layer = iota
	LayerUDPv4
	LayerUDPv6
)

const (
	event_port   = 319
	general_port = 320
)

type PacketData struct {
	IsTx     bool
	Packet   ptp.Packet
	HwTstamp time.Time
	SwTstamp time.Time
}

func (pd *PacketData) GetHeader() *ptp.Header {
	msgtype := pd.Packet.MessageType()
	switch msgtype {
	case ptp.MessageSync, ptp.MessageDelayReq:
		pkt := pd.Packet.(*ptp.SyncDelayReq)
		return &pkt.Header
	case ptp.MessagePDelayReq:
		pkt := pd.Packet.(*ptp.PDelayReq)
		return &pkt.Header
	case ptp.MessagePDelayResp:
		pkt := pd.Packet.(*ptp.PDelayResp)
		return &pkt.Header
	case ptp.MessageFollowUp:
		pkt := pd.Packet.(*ptp.FollowUp)
		return &pkt.Header
	case ptp.MessagePDelayRespFollowUp:
		pkt := pd.Packet.(*ptp.PDelayRespFollowUp)
		return &pkt.Header
	case ptp.MessageAnnounce:
		pkt := pd.Packet.(*ptp.Announce)
		return &pkt.Header
	case ptp.MessageSignaling:
		pkt := pd.Packet.(*ptp.Signaling)
		return &pkt.Header
	case ptp.MessageManagement:
		pkt := pd.Packet.(*ptp.Management)
		return &pkt.Header
	default:
		fmt.Printf("Invalid message type\n")
		return nil
	}
}

func (pd *PacketData) Print() {
	msgtype := pd.Packet.MessageType()
	hdr := pd.GetHeader()
	rx_ns := pd.HwTstamp.UnixNano() % 1000000000
	rx_s := pd.HwTstamp.Unix()
	seq := hdr.SequenceID
	corr := hdr.CorrectionField.Duration()
	domain := hdr.DomainNumber
	var dir string
	if pd.IsTx {
		dir = fmt.Sprintf("%s ->", msgtype)
	} else {
		dir = fmt.Sprintf("<- %s", msgtype)
	}
	fmt.Printf("%s | Seq %d | Dom %d | hwts %d.%09d | Corr %d\n", dir, seq, domain, rx_s, rx_ns, corr)
}

type Options struct {
	TransportSpecific uint8
	TwoStepFlag_set   bool
	Human_readable    bool
	IngressLatency    uint64
	EgressLatency     uint64
	TwoStepFlag       bool
	Interval          time.Duration
	Pkttype           *ptp.MessageType
	Minor_version     uint8
	Domain            uint8
	Count             int
	Vlan              *int
	Prio              int
	Seq               uint16

	RxMode     bool
	Iface      string
	Delay_mode string
	Clk_type   string
	Udp        bool
	Ip         string
	DestIp     string
	Mac        string

	RecordPackets bool

	// tstamp_all        int
	// auto_fup          int
	// listen   int
	// unsigned char mac[ETH_ALEN];
	// char *interface;
	// int sequence_types[SEQUENCE_MAX];
	// int sequence_length;

	// TODO: time/timestamp needs to be patched to support this.
	// enum delay_mechanism dm;
	// enum hwtstamp_clk_types clk_type;
	// header_offset     uint

}

type Port struct {
	IfaceStr      string
	IP            net.IP
	DestIP        net.IP
	Interface     *net.Interface
	Layer         Layer
	EFd           int
	txRecord      []PacketData
	rxRecord      []PacketData
	RecordPackets bool
	opts          Options
	portIdentity  ptp.PortIdentity
}

func (port *Port) recordRx(data PacketData) {
	if !port.RecordPackets {
		return
	}
	port.rxRecord = append(port.rxRecord, data)
}

func (port *Port) recordTx(data PacketData) {
	if !port.RecordPackets {
		return
	}
	port.txRecord = append(port.txRecord, data)
}

func (port *Port) receive(buf []byte, oob []byte) (*PacketData, error) {
	switch port.Layer {
	case LayerMac:
		bytes, _, hwts, err := timestamp.ReadPacketWithRXTimestampBuf(port.EFd, buf, oob)
		if err != nil {
			return nil, err
		}
		swts := time.Now()
		p, err := ptp.DecodePacket(buf[14:bytes])
		if err != nil {
			return nil, err
		}
		data := &PacketData{
			Packet:   p,
			HwTstamp: hwts,
			SwTstamp: swts,
			IsTx:     false,
		}
		port.recordRx(*data)
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

func (port *Port) ReceiveOne(opts *Options) (*PacketData, error) {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	pd, err := port.receive(buf, oob)
	if err != nil {
		return nil, err
	}
	return pd, nil
}

func (port *Port) RxMode(ch chan PacketData, quit chan int) {

	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	for {
		select {
		case _ = <-quit:
			close(ch)
			return
		default:
		}

		pd, err := port.receive(buf, oob)
		if err != nil {
			continue
		}
		ch <- *pd
	}
}

// TODO: Replace with proper function, this assumes host is LE
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

func (port *Port) transmit(pkt *ptp.Packet) error {
	// buf := make([]byte, timestamp.PayloadSizeBytes)

	// n, err := ptp.BytesTo(*pkt, buf)
	bytes, err := ptp.Bytes(*pkt)
	// n = n - 2 // Trim the unused TLV
	if err != nil {
		log.Fatalf("Failed to generate the sync packet: %v", err)
	}

	if port.Layer == LayerMac {
		hdr := []byte{0x01, 0x1b, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xaa, 0xaa, 0xaa, 0x88, 0xf7}
		// packet := append(hdr, buf[:n]...)
		packet := append(hdr, bytes...)
		// fmt.Println(len(packet), n)
		var addr syscall.SockaddrLinklayer
		addr.Protocol = syscall.ETH_P_1588
		addr.Ifindex = port.Interface.Index
		addr.Hatype = syscall.ARPHRD_ETHER

		err = syscall.Sendto(port.EFd, packet, 0, &addr)
	} else if port.Layer == LayerUDPv4 {
		eclisa := timestamp.IPToSockaddr(port.DestIP, event_port)
		// err = unix.Sendto(port.EFd, buf[:n], 0, eclisa)
		err = unix.Sendto(port.EFd, bytes, 0, eclisa)
	} else {
		log.Fatal("transmit: not implemented")
	}
	return err
}

func (port *Port) transmit_get_ts(pkt *ptp.Packet, oob []byte, toob []byte) (*time.Time, *time.Time, error) {
	// port.txRecord = append(port.txRecord, pkt)
	err := port.transmit(pkt)
	swtx := time.Now()
	if err != nil {
		return nil, nil, err
	}
	hwtx, _, err := timestamp.ReadTXtimestampBuf(port.EFd, oob, toob)
	if err != nil {
		return nil, nil, err
	}
	return &hwtx, &swtx, nil
}

func (port *Port) TxMode(input, output chan PacketData, quit chan int) {
	oob := make([]byte, timestamp.ControlSizeBytes)
	toob := make([]byte, timestamp.ControlSizeBytes)
	for pd := range input {
		hwts, swts, err := port.transmit_get_ts(&pd.Packet, oob, toob)
		if err != nil {
			fmt.Printf("Error %s\n", err)
			continue
		}
		// fmt.Printf("Timestamp %d\n", hwts.UnixNano())
		pd.HwTstamp = *hwts
		pd.SwTstamp = *swts
		port.recordTx(pd)
		output <- pd
	}
	close(output)
}

// func (port *Port) TxMode() {
// 	// syncP := &ptp.SyncDelayReq{
// 	// 	Header: ptp.Header{
// 	// 		SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0),
// 	// 		Version:         ptp.Version,
// 	// 		MessageLength:   uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})), //#nosec G115
// 	// 		DomainNumber:    opts.Domain,
// 	// 		FlagField:       ptp.FlagTwoStep,
// 	// 		SequenceID:      opts.Seq,
// 	// 		SourcePortIdentity: ptp.PortIdentity{
// 	// 			PortNumber:    1,
// 	// 			ClockIdentity: 0x000000fffeaa0000,
// 	// 		},
// 	// 		LogMessageInterval: 0,
// 	// 		ControlField:       0,
// 	// 	},
// 	// }
// 	pkt, err := port.buildPacket(ptp.MessageSync, port.opts.Seq)
// 	if err != nil {
// 		log.Fatalf("Failed building packet: %s", err)
// 	}

// 	seq := port.opts.Seq
// 	oob := make([]byte, timestamp.ControlSizeBytes)
// 	toob := make([]byte, timestamp.ControlSizeBytes)
// 	for i := range port.opts.Count {
// 		// err := port.transmit(syncP)
// 		// if err != nil {
// 		// 	continue
// 		// }
// 		// txTS, _, err := timestamp.ReadTXtimestampBuf(port.EFd, oob, toob)
// 		// if err != nil {
// 		// 	continue
// 		// }
// 		hwts, swts, err := port.transmit_get_ts(&pkt, oob, toob)
// 		data := PacketData{
// 			Packet:   pkt,
// 			HwTstamp: *hwts,
// 			SwTstamp: *swts,
// 		}
// 		port.recordTx(data)
// 		seq += 1
// 		pkt.SetSequence(seq)
// 		if err != nil {
// 			continue
// 		}
// 		// fmt.Println(swts.UnixNano())
// 		tx_ns := data.HwTstamp.UnixNano() % 1000000000
// 		tx_s := data.HwTstamp.Unix()
// 		fmt.Printf("hwts %d.%09d\n", tx_s, tx_ns)
// 		// fmt.Println(hwts.UnixNano())
// 		if i+1 != port.opts.Count {
// 			time.Sleep(port.opts.Interval)
// 		}
// 	}
// 	// fmt.Printf("%v\n", port.txRecord)
// }

func InterfaceFromIP(ipStr string) (string, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", ipStr)
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var currentIP net.IP

			switch v := addr.(type) {
			case *net.IPNet:
				currentIP = v.IP
			case *net.IPAddr:
				currentIP = v.IP
			}

			if currentIP.Equal(ip) {
				return iface.Name, nil
			}
		}
	}

	return "", fmt.Errorf("no interface found for IP %s", ipStr)
}

func (port *Port) Init(opts Options, clockid uint16, portnum uint16) error {
	if opts.Udp {
		ip := net.ParseIP(opts.Ip)
		dest := net.ParseIP(opts.DestIp)
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
	// Use portnum in clockid to make it unique for each port since we will
	// never run as a BC/TC.
	// TODO: Should other instances use other portnums?
	port.portIdentity = ptp.PortIdentity{
		PortNumber:    portnum,
		ClockIdentity: 0xbeef00fffeaa0000 + ptp.ClockIdentity(clockid),
	}
	port.opts = opts
	err := port.openSocket()
	if err != nil {
		return err
	}

	// Enable RX timestamps. Delay requests need to be timestamped by ptp4u on receipt
	netif, err := net.InterfaceByName(port.IfaceStr)
	port.Interface = netif
	if err != nil {
		fmt.Printf("Failed fetching interface\n")
		return err
	}
	if err := timestamp.EnableTimestamps(timestamp.SW, port.EFd, netif); err != nil {
		fmt.Printf("Failed enabling timestamps\n")
		return err
	}

	err = unix.SetNonblock(port.EFd, false)
	if err != nil {
		fmt.Printf("Failed to set socket to blocking\n")
		return err
	}

	tmo := unix.Timeval{
		Sec:  0,
		Usec: 100000, // 100 ms
	}
	unix.SetsockoptTimeval(port.EFd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tmo)
	return nil
}

func (port *Port) BuildPacket(msgtype ptp.MessageType, seq uint16) (ptp.Packet, error) {
	hdr := ptp.Header{
		SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0),
		Version:            ptp.Version,
		MessageLength:      uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})), //#nosec G115
		DomainNumber:       port.opts.Domain,
		FlagField:          ptp.FlagTwoStep, // TODO: check mode
		SequenceID:         seq,
		SourcePortIdentity: port.portIdentity,
		LogMessageInterval: 0,
		ControlField:       0,
	}
	var p ptp.Packet
	switch msgtype {
	case ptp.MessageSync, ptp.MessageDelayReq:
		p = &ptp.SyncDelayReq{Header: hdr}
	case ptp.MessagePDelayReq:
		p = &ptp.PDelayReq{Header: hdr}
	case ptp.MessagePDelayResp:
		p = &ptp.PDelayResp{Header: hdr}
	case ptp.MessageFollowUp:
		p = &ptp.FollowUp{Header: hdr}
	case ptp.MessageDelayResp:
		p = &ptp.DelayResp{Header: hdr}
	case ptp.MessagePDelayRespFollowUp:
		p = &ptp.PDelayRespFollowUp{Header: hdr}
	case ptp.MessageAnnounce:
		p = &ptp.Announce{Header: hdr}
	case ptp.MessageSignaling:
		p = &ptp.Signaling{Header: hdr}
	// case ptp.MessageManagement:
	// p = &ptp.Management{Header: hdr}
	default:
		return nil, fmt.Errorf("unsupported type %s", msgtype)
	}
	return p, nil
}

// func (pkt *ptp.Packet) ToBytes() ([]bytes, n) {

// }
