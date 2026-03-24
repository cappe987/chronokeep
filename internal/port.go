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
	Packet   ptp.Packet
	Rxtstamp time.Time
}

type Port struct {
	IfaceStr  string
	IP        net.IP
	DestIP    net.IP
	Interface *net.Interface
	Layer     Layer
	EFd       int
}

type Options struct {
	TransportSpecific uint8
	TwoStepFlag_set   bool
	Human_readable    bool
	IngressLatency    uint64
	EgressLatency     uint64
	Nonstop_flag      bool
	TwoStepFlag       bool
	Interval          time.Duration
	Pkttype           *ptp.MessageType
	Minor_version     uint8
	Domain            uint8
	Count             int
	Vlan              *int
	Prio              int
	Seq               uint16

	Rx_mode    bool
	Iface      string
	Delay_mode string
	Clk_type   string
	Udp        bool

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

func (port *Port) receive(buf []byte, oob []byte) (*PacketData, error) {
	switch port.Layer {
	case LayerMac:
		bytes, _, rxTS, err := timestamp.ReadPacketWithRXTimestampBuf(port.EFd, buf, oob)
		if err != nil {
			return nil, err
		}
		p, err := ptp.DecodePacket(buf[14:bytes])
		if err != nil {
			return nil, err
		}
		data := &PacketData{
			Packet:   p,
			Rxtstamp: rxTS,
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

func (port *Port) ReceiveOne(opts *Options) (*PacketData, error) {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	pd, err := port.receive(buf, oob)
	if err != nil {
		return nil, err
	}
	return pd, nil
}

func (port *Port) RxMode(opts *Options, ch chan PacketData, quit chan int) {

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

func (p *Port) OpenSocket() error {
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

func (port *Port) TxMode(opts *Options) {
	syncP := &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})), //#nosec G115
			DomainNumber:    opts.Domain,
			FlagField:       ptp.FlagTwoStep,
			SequenceID:      opts.Seq,
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
	for _ = range opts.Count {
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
		time.Sleep(opts.Interval)
	}
}
