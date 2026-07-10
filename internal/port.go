package internal

import (
	"fmt"
	"log"
	"net"
	"syscall"
	"time"
	"unsafe"

	ptp "github.com/facebook/time/ptp/protocol"
	timestamp "github.com/facebook/time/timestamp"
	"golang.org/x/net/bpf"
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
	efd           int
	gfd           int
	txRecord      []PacketData
	rxRecord      []PacketData
	RecordPackets bool
	portIdentity  ptp.PortIdentity
	syncSeq       uint16
	annoSeq       uint16
	delaySeq      uint16
	opts          CommonOpts
	PortOpts      PortOpts
	App           *App
	Silent        bool
}

func (port *Port) EnableRecording() {
	port.opts.RecordPackets = true
	port.RecordPackets = true
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

func (port *Port) doReceive(buf []byte, oob []byte, getTs bool) (int, time.Time, error) {
	if getTs {
		bytes, _, hwts, err := timestamp.ReadPacketWithRXTimestampBuf(port.efd, buf, oob)
		return bytes, hwts, err
	} else {
		bytes, _, err := unix.Recvfrom(port.gfd, buf, 0)
		hwts := time.Unix(0, 0)
		return bytes, hwts, err
	}
}

func (port *Port) receive(buf []byte, oob []byte, getTs bool) (*PacketData, error) {
	switch port.Layer {
	case LayerMac:
		bytes, hwts, err := port.doReceive(buf, oob, getTs)
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
			Iface:    port.IfaceStr,
		}
		hdr := data.GetHeader()
		if hdr.DomainNumber != port.opts.Domain {
			return nil, fmt.Errorf("Wrong domain, got %d", hdr.DomainNumber)
		}
		if data.PidEquals(port.portIdentity) {
			return nil, fmt.Errorf("Packet sent by self")
		}
		// port.recordRx(*data)
		return data, nil
	case LayerUDPv4:
		// bytes, _, hwts, err := timestamp.ReadPacketWithRXTimestampBuf(port.efd, buf, oob)
		bytes, hwts, err := port.doReceive(buf, oob, getTs)
		if err != nil {
			return nil, err
		}
		swts := time.Now()
		p, err := ptp.DecodePacket(buf[:bytes])
		if err != nil {
			return nil, err
		}
		data := &PacketData{
			Packet:   p,
			HwTstamp: hwts,
			SwTstamp: swts,
			IsTx:     false,
			Iface:    port.IfaceStr,
		}
		hdr := data.GetHeader()
		if hdr.DomainNumber != port.opts.Domain {
			return nil, fmt.Errorf("Wrong domain, got %d", hdr.DomainNumber)
		}
		// port.recordRx(*data)
		return data, nil
	default:
		log.Fatal("receive: not implemented")
	}
	return nil, nil
}

func (port *Port) receive_get_ts(buf []byte, oob []byte) (*PacketData, error) {
	return port.receive(buf, oob, true)
}
func (port *Port) receive_no_ts(buf []byte, oob []byte) (*PacketData, error) {
	return port.receive(buf, oob, false)
}

func (port *Port) ReceiveOne() (*PacketData, error) {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	pd, err := port.receive_get_ts(buf, oob)
	if err != nil {
		return nil, err
	}
	return pd, nil
}

func (port *Port) rxEvent(ch chan PacketData, quit chan int) {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	for {
		select {
		case _ = <-quit:
			close(ch)
			return
		default:
		}
		pd, err := port.receive_get_ts(buf, oob)
		if err != nil {
			continue
		}
		ch <- *pd
	}
}

func (port *Port) rxGeneral(ch chan PacketData, quit chan int) {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	for {
		select {
		case _ = <-quit:
			close(ch)
			return
		default:
		}
		pd, err := port.receive_no_ts(buf, oob)
		if err != nil {
			continue
		}
		ch <- *pd
	}
}

// Receive packets from each channel, record it, and send on the common channel
func (port *Port) RxMode(ch chan PacketData, quit chan int) {
	eventCh := make(chan PacketData, 100)
	genCh := make(chan PacketData, 100)
	go port.rxEvent(eventCh, quit)
	go port.rxGeneral(genCh, quit)
	for {
		select {
		case _ = <-quit:
			close(ch)
			return
		case pd := <-eventCh:
			port.recordRx(pd)
			port.ShowPacket(&pd)
			ch <- pd
		case pd := <-genCh:
			port.recordRx(pd)
			port.ShowPacket(&pd)
			ch <- pd
		}
	}
}

// https://riyazali.net/berkeley-packet-filter-in-golang
// Filter represents a classic BPF filter program that can be applied to a socket
type Filter []bpf.Instruction

// ApplyTo applies the current filter onto the provided file descriptor
func (filter Filter) ApplyTo(fd int) (err error) {
	var assembled []bpf.RawInstruction
	if assembled, err = bpf.Assemble(filter); err != nil {
		return err
	}

	var program = unix.SockFprog{
		Len:    uint16(len(assembled)),
		Filter: (*unix.SockFilter)(unsafe.Pointer(&assembled[0])),
	}
	var b = (*[unix.SizeofSockFprog]byte)(unsafe.Pointer(&program))[:unix.SizeofSockFprog]

	if _, _, errno := syscall.Syscall6(syscall.SYS_SETSOCKOPT,
		uintptr(fd), uintptr(syscall.SOL_SOCKET), uintptr(syscall.SO_ATTACH_FILTER),
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0); errno != 0 {
		return errno
	}

	return nil
}

// TODO: Handle VLAN tags. Handle majorSdoId
func (p *Port) addSocketFilter(isEventSocket bool) error {
	var eventFilter = Filter{
		bpf.LoadAbsolute{Off: 12, Size: 2},                        // load the ether protocol
		bpf.JumpIf{Val: 0x88f7, SkipTrue: 1},                      // if Val == 0x88f7 skip next instruction
		bpf.RetConstant{Val: 0x0},                                 // return 0 bytes, effectively ignore this packet
		bpf.LoadAbsolute{Off: 14, Size: 1},                        // load the majorSdoId/messageType
		bpf.JumpIf{Cond: bpf.JumpLessThan, Val: 0x4, SkipTrue: 1}, // if packet is event, skip next instruction. Does not handle majorSdoId != 0.
		bpf.RetConstant{Val: 0x0},                                 // return 0 bytes, effectively ignore this packet
		bpf.RetConstant{Val: 0xffff},                              // return 0xffff bytes (or less) from packet
	}
	var generalFilter = Filter{
		bpf.LoadAbsolute{Off: 12, Size: 2},                           // load the ether protocol
		bpf.JumpIf{Val: 0x88f7, SkipTrue: 1},                         // if Val == 0x88f7 skip next instruction
		bpf.RetConstant{Val: 0x0},                                    // return 0 bytes, effectively ignore this packet
		bpf.LoadAbsolute{Off: 14, Size: 1},                           // load the majorSdoId/messageType
		bpf.JumpIf{Cond: bpf.JumpGreaterThan, Val: 0x4, SkipTrue: 1}, // if packet is general, skip next instruction. Does not handle majorSdoId != 0.
		bpf.RetConstant{Val: 0x0},                                    // return 0 bytes, effectively ignore this packet
		bpf.RetConstant{Val: 0xffff},                                 // return 0xffff bytes (or less) from packet
	}

	if isEventSocket {
		return eventFilter.ApplyTo(p.efd)
	} else {
		return generalFilter.ApplyTo(p.gfd)
	}
}

// TODO: Replace with proper function, this assumes host is LE
func htons(i uint16) uint16 {
	return (i<<8)&0xff00 | i>>8
}

func (p *Port) openSocket(isEventSocket bool) error {
	if p.Layer == LayerMac {
		fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, syscall.ETH_P_ALL)
		if err != nil {
			return err
		}
		interf, err := net.InterfaceByName(p.IfaceStr)
		if err != nil {
			return err
		}
		p.Interface = interf
		if isEventSocket {
			p.efd = fd
		} else {
			p.gfd = fd
		}
		p.addSocketFilter(isEventSocket)

		sll := &syscall.SockaddrLinklayer{
			Protocol: htons(syscall.ETH_P_ALL),
			Ifindex:  interf.Index,
		}
		if err := syscall.Bind(fd, sll); err != nil {
			log.Fatalf("bind to %s failed: %v", p.IfaceStr, err)
		}

		// fmt.Printf("Opened L2 socket on %s\n", p.IfaceStr)
	} else if p.Layer == LayerUDPv4 {
		// TODO: Multicast is not working. net.ListenMulticastUDP probably
		var udp_port int
		if isEventSocket {
			udp_port = event_port
		} else {
			udp_port = general_port
		}
		conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: p.IP, Port: udp_port})
		if err != nil {
			log.Fatalf("Listening error: %s", err)
		}
		// defer conn.Close()
		// get connection file descriptor
		fd, err := timestamp.ConnFd(conn)
		if isEventSocket {
			p.efd = fd
		} else {
			p.gfd = fd
		}
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
		hdr := []byte{0x01, 0x1b, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xaa, 0xaa, 0xaa}
		ethertype := []byte{0x88, 0xf7}
		p := *pkt
		msgtype := p.MessageType()
		if msgtype == ptp.MessagePDelayReq || msgtype == ptp.MessagePDelayResp || msgtype == ptp.MessagePDelayRespFollowUp {
			hdr = []byte{0x01, 0x80, 0xC2, 0x00, 0x00, 0x0E, 0x00, 0x00, 0x00, 0xaa, 0xaa, 0xaa}
		}
		if port.opts.Vlan != nil {
			pcp := uint16(0)
			pcp = (uint16(port.opts.Prio) & 0x7) << 13
			vid := *port.opts.Vlan & 0x1fff
			val := uint16(pcp) | vid
			upper := uint8(val >> 8)
			lower := uint8(val)
			tag := []byte{0x81, 0x00, upper, lower}
			hdr = append(hdr, tag...)
		}
		hdr = append(hdr, ethertype...)
		// packet := append(hdr, buf[:n]...)
		packet := append(hdr, bytes...)
		// fmt.Println(len(packet), n)
		var addr syscall.SockaddrLinklayer
		addr.Protocol = syscall.ETH_P_1588
		addr.Ifindex = port.Interface.Index
		addr.Hatype = syscall.ARPHRD_ETHER

		err = syscall.Sendto(port.efd, packet, 0, &addr)
	} else if port.Layer == LayerUDPv4 {
		eclisa := timestamp.IPToSockaddr(port.DestIP, event_port)
		// err = unix.Sendto(port.efd, buf[:n], 0, eclisa)
		err = unix.Sendto(port.efd, bytes, 0, eclisa)
	} else {
		log.Fatal("transmit: not implemented")
	}
	return err
}

func (port *Port) transmit_get_ts(pkt *ptp.Packet, oob []byte, toob []byte) (*time.Time, *time.Time, error) {
	err := port.transmitPkt(pkt)
	swts := time.Now()
	if err != nil {
		return nil, nil, err
	}
	hwts, _, err := timestamp.ReadTXtimestampBuf(port.efd, oob, toob)
	if err != nil {
		return nil, nil, err
	}
	return &hwts, &swts, nil
}

func (port *Port) transmit_no_ts(pkt *ptp.Packet, oob []byte, toob []byte) (*time.Time, *time.Time, error) {
	err := port.transmitPkt(pkt)
	swts := time.Now()
	if err != nil {
		return nil, nil, err
	}
	hwts := time.Unix(0, 0)
	return &hwts, &swts, nil
}

// TODO: Handle event vs general packets. Now everything expects timestamp
func (port *Port) Transmit(pd *PacketData) *PacketData {
	oob := make([]byte, timestamp.ControlSizeBytes)
	toob := make([]byte, timestamp.ControlSizeBytes)
	var hwts *time.Time
	var swts *time.Time
	var err error = nil
	if pd.IsNonTimestampPacket() {
		hwts, swts, err = port.transmit_no_ts(&pd.Packet, oob, toob)
	} else {
		hwts, swts, err = port.transmit_get_ts(&pd.Packet, oob, toob)
	}
	if err != nil {
		fmt.Printf("Error %s\n", err)
		return nil
	}
	// fmt.Printf("Timestamp %d\n", hwts.UnixNano())
	pd.HwTstamp = *hwts
	pd.SwTstamp = *swts
	pd.Iface = port.IfaceStr
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

func (port *Port) Init(app *App, clockid uint16, portnum uint16) error {
	if app.Opts.Udp {
		ip := net.ParseIP(app.Opts.Ip)
		dest := net.ParseIP(app.Opts.DestIp)
		port.Layer = LayerUDPv4
		port.IP = ip
		port.DestIP = dest
	} else {
		port.Layer = LayerMac
	}
	if app.Opts.Iface != "" && app.Opts.Iface != "dummy" {
		port.IfaceStr = app.Opts.Iface
	}
	port.RecordPackets = app.Opts.RecordPackets
	// Use portnum in clockid to make it unique for each port since we will
	// never run as a BC/TC.
	// TODO: Should other instances use other portnums?
	port.portIdentity = ptp.PortIdentity{
		PortNumber:    portnum,
		ClockIdentity: 0xbeef00fffeaa0000 + ptp.ClockIdentity(clockid),
	}
	port.opts = app.Opts
	port.App = app
	err := port.openSocket(true)
	if err != nil {
		return err
	}
	err = port.openSocket(false)
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
	if app.Opts.SwTstamp {
		tstamp = timestamp.SW
	} else if app.Opts.Onestep {
		tstamp = timestamp.HWONESTEP
	}
	if err := timestamp.EnableTimestamps(tstamp, port.efd, netif); err != nil {
		fmt.Printf("Failed enabling timestamps\n")
		return err
	}

	err = unix.SetNonblock(port.efd, false)
	if err != nil {
		fmt.Printf("Failed to set socket to blocking\n")
		return err
	}
	err = unix.SetNonblock(port.gfd, false)
	if err != nil {
		fmt.Printf("Failed to set socket to blocking\n")
		return err
	}

	tmo := unix.Timeval{
		Sec:  0,
		Usec: 100000, // 100 ms
	}
	unix.SetsockoptTimeval(port.efd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tmo)
	unix.SetsockoptTimeval(port.gfd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tmo)
	return nil
}

func (port *Port) Deinit() {
	timestamp.DisableTimestamps(port.efd, port.Interface)
}

func (port *Port) GetVersion() uint8 {
	return (port.opts.MinorVersion << 4) | ptp.MajorVersion
}

func (port *Port) ShowPacket(pd *PacketData) {
	if port.Silent {
		return
	}
	if port.App.Cli {
		pd.Print()
	} else if port.App.WsOut != nil {
		port.App.WsOut <- *pd
	}
}

func (port *Port) TransmitSyncFup() {
	sync := port.BuildSync(port.syncSeq)
	port.Transmit(sync)
	port.ShowPacket(sync)
	port.syncSeq += 1
	if !port.opts.Onestep {
		fup := port.MakeFollowUp(sync)
		port.Transmit(fup)
		port.ShowPacket(fup)
	}
}

func (port *Port) TransmitAnnounce() {
	anno := port.BuildAnnounce(port.annoSeq)
	port.Transmit(anno)
	port.ShowPacket(anno)
	port.annoSeq += 1
}

func (port *Port) TransmitPDelayReq() {
	pdelayReq := port.BuildPDelayReq(port.delaySeq)
	port.Transmit(pdelayReq)
	port.ShowPacket(pdelayReq)
	port.delaySeq += 1
}

func (port *Port) TransmitDelayReq() {
	delayReq := port.BuildDelayReq(port.delaySeq)
	port.Transmit(delayReq)
	port.ShowPacket(delayReq)
	port.delaySeq += 1
}

func (port *Port) ReplyToDelayReq(pd *PacketData) {
	resp := port.MakeResponseDelay(pd)
	port.Transmit(resp)
	port.ShowPacket(resp)
}
func (port *Port) ReplyToPDelayReq(pd *PacketData) {
	resp := port.MakeResponsePDelay(pd)
	port.Transmit(resp)
	port.ShowPacket(resp)
	respFup := port.MakeFollowUpPDelay(resp)
	port.Transmit(respFup)
	port.ShowPacket(respFup)
}
