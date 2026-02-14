package main

import (
	//"bytes"
	"encoding/binary"
	//"encoding/hex"
	"fmt"
	"log"
	"net"
	"syscall"
	"time"
	"flag"

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

		fmt.Printf("Opened L2 socket on %s\n", p.IfaceStr)
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

func (port *Port) receive() error {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	// toob := make([]byte, timestamp.ControlSizeBytes)

	switch port.Layer {
	case LayerMac:
		
		//n, from, err := syscall.Recvfrom(port.EFd, buf, 0)
		//if err != nil {
			//log.Fatalf("recvfrom failed: %v", err)
		//}
		//fmt.Printf("Received %d bytes from %+v\n", n, from)
		//fmt.Println(hex.Dump(buf[:n]))

		//_, _, _, saddr, err := unix.Recvmsg(port.EFd, buf, oob, 0)
		//if err != nil {
			//return err
		//}
		//fmt.Printf("saddr %v\n", saddr)
		//_, _, rxTS, err := timestamp.ReadPacketWithRXTimestampBuf(port.EFd, buf, oob)
		//data, returnaddr, nowKernelTimestamp, err := ReadPacketWithRXTimestamp(connFd)
		_, _, rxTS, err := timestamp.ReadPacketWithRXTimestamp(port.EFd)
		if err != nil {
			return err
		}
		fmt.Println("RX TS:", rxTS.UnixNano())
		//log.Fatal("receive: not implemented")
	case LayerUDPv4:
		_, _, rxTS, err := timestamp.ReadPacketWithRXTimestampBuf(port.EFd, buf, oob)
		if err != nil {
			return err
		}
		fmt.Println("RX TS:", rxTS.UnixNano())
	default:
		log.Fatal("receive: not implemented")
	}
	return nil
}

func main() {
	// port := 319
	// dest := net.IPv4(10, 11, 0, 2)
	//use_l2 := true
	var (
		// eFd    int
		// err    error
		// interf *net.Interface
		port Port
		txTS time.Time
	)
	p1 := false

	var txport = flag.String("tx", "", "help message for flag n")
	var rxport = flag.String("rx", "", "help message for flag n")
	var l2 = flag.Bool("2", false, "help message for flag n")

	flag.Parse()

	fmt.Printf("TX: %s. RX: %s. L2: %v\n", *txport, *rxport, *l2)

	if *txport == "" && *rxport == "" {
		return
	}

	if *txport != "" {
		p1 = true
	}


	if p1 {
		ip := net.IPv4(10, 11, 0, 1)
		// dest := net.IPv4(224, 0, 1, 129)
		dest := net.IPv4(10, 11, 0, 2)
		iface := *txport
		if *l2 {
			port.IfaceStr = iface
			port.Layer = LayerMac
		} else {
			port.IfaceStr = iface
			port.Layer = LayerUDPv4
			port.IP = ip
			port.DestIP = dest
		}
	} else {
		ip := net.IPv4(10, 11, 0, 2)
		dest := net.IPv4(10, 11, 0, 1)
		iface := *rxport
		if *l2 {
			port.IfaceStr = iface
			port.Layer = LayerMac
		} else {
			port.IfaceStr = iface
			port.Layer = LayerUDPv4
			port.IP = ip
			port.DestIP = dest
		}
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

	err := port.openSocket()

	if err != nil {
		log.Fatalf("Getting event connection FD: %s", err)
	}

	// Enable RX timestamps. Delay requests need to be timestamped by ptp4u on receipt
	if err := timestamp.EnableTimestamps(timestamp.SW, port.EFd, port.IfaceStr); err != nil {
		log.Fatal(err)
	}

	err = unix.SetNonblock(port.EFd, false)
	if err != nil {
		log.Fatalf("Failed to set socket to blocking: %s", err)
	}

	syncP := &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})), //#nosec G115
			DomainNumber:    0,
			FlagField:       ptp.FlagTwoStep,
			SequenceID:      0,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: 0x000000fffeaa0000,
			},
			LogMessageInterval: 0,
			ControlField:       0,
		},
	}
	oob := make([]byte, timestamp.ControlSizeBytes)
	// TMP buffers
	toob := make([]byte, timestamp.ControlSizeBytes)

	if p1 {
		err = port.transmit(syncP)
		txTS, _, err = timestamp.ReadTXtimestampBuf(port.EFd, oob, toob)
		fmt.Println(txTS.UnixNano())
	} else {
		port.receive()
	}

	// err = unix.Sendto(eFd, buf[:n], 0, eclisa)
	// txTS, _, err = timestamp.ReadTXtimestampBuf(eFd, oob, toob)
	// fmt.Println(txTS.UnixNano())

	// err = unix.Sendto(eFd, buf[:n], 0, eclisa)
	// txTS, _, err = timestamp.ReadTXtimestampBuf(eFd, oob, toob)
	// fmt.Println(txTS.UnixNano())
}
