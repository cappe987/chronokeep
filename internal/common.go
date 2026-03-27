package internal

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/pborman/getopt/v2"
)

type CommonOpts struct {
	TransportSpecific uint8
	IngressLatency    uint64
	EgressLatency     uint64
	MinorVersion      uint8
	Domain            uint8
	Vlan              *int
	Prio              int
	Udp               bool
	Iface             string
	Ip                string
	DestIp            string
	Mac               string
	RecordPackets     bool
	Help              bool
	SwTstamp          bool
	Onestep           bool
}

func (opts *CommonOpts) DefineCommonFlags() {
	// TODO: IP multicast not working yet
	opts.DestIp = "224.0.1.129" // TODO: 224.0.0.107 for pdelays
	opts.Mac = "01:1b:19:00:00:00"
	opts.RecordPackets = true

	getopt.FlagLong(&opts.Domain, "domain", 'd', "PTP domain")
	getopt.FlagLong(&opts.Iface, "iface", 'i', "Interface to operate on")
	getopt.FlagLong(&opts.IngressLatency, "ilat", 0, "Ingress latency (ns)")
	getopt.FlagLong(&opts.EgressLatency, "elat", 0, "Egress latency (ns)")
	getopt.FlagLong(&opts.Udp, "udp", '4', "Use IPv4 UDP instead of L2")
	getopt.FlagLong(&opts.Ip, "sip", 0, "Source IP for UDP mode")
	getopt.FlagLong(&opts.DestIp, "dip", 0, "Destination IP for UDP mode")
	getopt.FlagLong(&opts.Mac, "mac", 'm', "Destination MAC for L2 mode. P2P always uses link-local")
	getopt.FlagLong(&opts.Help, "help", 'h', "Show this help menu")
	getopt.FlagLong(&opts.SwTstamp, "swtstamp", 'S', "Software timestamping")
	getopt.FlagLong(&opts.Onestep, "onestep", 'o', "Onestep timestamping")
}

func (opts *CommonOpts) Validate() {
	if opts.Help {
		getopt.Usage()
		os.Exit(1)
	}

	if opts.Udp {
		if opts.Ip == "" {
			log.Fatalf("Must specify source IP with --sip\n")
		} else if opts.Iface == "" {
			iface, err := InterfaceFromIP(opts.Ip)
			if err != nil {
				getopt.Usage()
				log.Fatalf("Unable to find interface with IP %s\n", opts.Ip)
			}
			opts.Iface = iface
		}
	} else {
		if opts.Iface == "" {
			getopt.Usage()
			log.Fatalf("Must specify interface with --iface")

		}
	}
}

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

func StringToMessageType(str string) ptp.MessageType {
	switch strings.ToLower(str) {
	case "sync":
		return ptp.MessageSync
	case "delay_req":
		return ptp.MessageDelayReq
	case "pdelay_req":
		return ptp.MessagePDelayReq
	case "pdelay_resp":
		return ptp.MessagePDelayResp
	case "follow_up":
		return ptp.MessageFollowUp
	case "delay_resp":
		return ptp.MessageDelayResp
	case "pdelay_resp_follow_up":
		return ptp.MessagePDelayRespFollowUp
	case "announce":
		return ptp.MessageAnnounce
	case "signaling":
		return ptp.MessageSignaling
	case "management":
		return ptp.MessageManagement
	}
	log.Fatalf("Invalid message type: %s", str)
	panic("Invalid message type")
}
