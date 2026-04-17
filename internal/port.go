package internal

import (
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
	opts          CommonOpts
	portIdentity  ptp.PortIdentity
	syncSeq       uint16
	annoSeq       uint16
	delaySeq      uint16
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
		hdr := data.GetHeader()
		if hdr.DomainNumber != port.opts.Domain {
			return nil, fmt.Errorf("Wrong domain, got %d", hdr.DomainNumber)
		}
		port.recordRx(*data)
		return data, nil
	case LayerUDPv4:
		bytes, _, hwts, err := timestamp.ReadPacketWithRXTimestampBuf(port.EFd, buf, oob)
		swts := time.Now()
		if err != nil {
			return nil, err
		}
		p, err := ptp.DecodePacket(buf[:bytes])
		if err != nil {
			return nil, err
		}
		data := &PacketData{
			Packet:   p,
			HwTstamp: hwts,
			SwTstamp: swts,
			IsTx:     false,
		}
		hdr := data.GetHeader()
		if hdr.DomainNumber != port.opts.Domain {
			return nil, fmt.Errorf("Wrong domain, got %d", hdr.DomainNumber)
		}
		port.recordRx(*data)
		return data, nil
	default:
		log.Fatal("receive: not implemented")
	}
	return nil, nil
}

func (port *Port) ReceiveOne() (*PacketData, error) {
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
		// TODO: Multicast is not working. net.ListenMulticastUDP probably
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

func (port *Port) transmitPkt(pkt *ptp.Packet) error {
	// buf := make([]byte, timestamp.PayloadSizeBytes)

	// n, err := ptp.BytesTo(*pkt, buf)
	bytes, err := ptp.Bytes(*pkt)
	bytes = bytes[:len(bytes)-2]
	// n = n - 2 // Trim the unused TLV
	if err != nil {
		log.Fatalf("Failed to generate the sync packet: %v", err)
	}

	if port.Layer == LayerMac {
		hdr := []byte{0x01, 0x1b, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xaa, 0xaa, 0xaa, 0x88, 0xf7}
		p := *pkt
		msgtype := p.MessageType()
		if msgtype == ptp.MessagePDelayReq || msgtype == ptp.MessagePDelayResp || msgtype == ptp.MessagePDelayRespFollowUp {
			hdr = []byte{0x01, 0x80, 0xC2, 0x00, 0x00, 0x0E, 0x00, 0x00, 0x00, 0xaa, 0xaa, 0xaa, 0x88, 0xf7}
		}
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
	err := port.transmitPkt(pkt)
	swts := time.Now()
	if err != nil {
		return nil, nil, err
	}
	hwts, _, err := timestamp.ReadTXtimestampBuf(port.EFd, oob, toob)
	if err != nil {
		return nil, nil, err
	}
	return &hwts, &swts, nil
}

// TODO: Handle event vs general packets. Now everything expects timestamp
func (port *Port) Transmit(pd *PacketData) *PacketData {
	oob := make([]byte, timestamp.ControlSizeBytes)
	toob := make([]byte, timestamp.ControlSizeBytes)
	hwts, swts, err := port.transmit_get_ts(&pd.Packet, oob, toob)
	if err != nil {
		fmt.Printf("Error %s\n", err)
		return nil
	}
	// fmt.Printf("Timestamp %d\n", hwts.UnixNano())
	pd.HwTstamp = *hwts
	pd.SwTstamp = *swts
	port.recordTx(*pd)
	return pd
}

// func (port *Port) TxMode(input, output chan PacketData, quit chan int) {
// 	oob := make([]byte, timestamp.ControlSizeBytes)
// 	toob := make([]byte, timestamp.ControlSizeBytes)
// 	for pd := range input {
// 		hwts, swts, err := port.transmit_get_ts(&pd.Packet, oob, toob)
// 		if err != nil {
// 			fmt.Printf("Error %s\n", err)
// 			continue
// 		}
// 		// fmt.Printf("Timestamp %d\n", hwts.UnixNano())
// 		pd.HwTstamp = *hwts
// 		pd.SwTstamp = *swts
// 		port.recordTx(pd)
// 		output <- pd
// 	}
// 	close(output)
// }

func (port *Port) Init(opts CommonOpts, clockid uint16, portnum uint16) error {
	if opts.Udp {
		ip := net.ParseIP(opts.Ip)
		dest := net.ParseIP(opts.DestIp)
		port.Layer = LayerUDPv4
		port.IP = ip
		port.DestIP = dest
	} else {
		port.Layer = LayerMac
	}
	port.IfaceStr = opts.Iface
	port.RecordPackets = true
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

	netif, err := net.InterfaceByName(port.IfaceStr)
	port.Interface = netif
	if err != nil {
		fmt.Printf("Failed fetching interface\n")
		return err
	}
	tstamp := timestamp.HW
	if opts.SwTstamp {
		tstamp = timestamp.SW
	} else if opts.Onestep {
		tstamp = timestamp.HWONESTEP
	}
	if err := timestamp.EnableTimestamps(tstamp, port.EFd, netif); err != nil {
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

func (port *Port) Deinit() {
	timestamp.DisableTimestamps(port.EFd, port.Interface)
}

func (port *Port) GetVersion() uint8 {
	return (port.opts.MinorVersion << 4) | ptp.MajorVersion
}

func (port *Port) TransmitSyncFup() {
	sync := port.BuildSync(port.syncSeq)
	port.Transmit(sync)
	sync.Print()
	port.syncSeq += 1
	if !port.opts.Onestep {
		fup := port.MakeFollowUp(sync)
		port.Transmit(fup)
		fup.Print()
	}
}

func (port *Port) TransmitAnnounce() {
	anno := port.BuildAnnounce(port.annoSeq)
	port.Transmit(anno)
	anno.Print()
	port.annoSeq += 1
}

func (port *Port) TransmitPDelayReq() {
	pdelayReq := port.BuildPDelayReq(port.delaySeq)
	port.Transmit(pdelayReq)
	pdelayReq.Print()
	port.delaySeq += 1
}

func (port *Port) TransmitDelayReq() {
	delayReq := port.BuildDelayReq(port.delaySeq)
	port.Transmit(delayReq)
	delayReq.Print()
	port.delaySeq += 1
}

func (port *Port) ReplyToDelayReq(pd *PacketData) {
	resp := port.MakeResponseDelay(pd)
	port.Transmit(resp)
	resp.Print()
}
func (port *Port) ReplyToPDelayReq(pd *PacketData) {
	resp := port.MakeResponsePDelay(pd)
	port.Transmit(resp)
	resp.Print()
	respFup := port.MakeFollowUpPDelay(resp)
	port.Transmit(respFup)
	respFup.Print()
}
