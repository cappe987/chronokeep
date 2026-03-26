package internal

import (
	"fmt"
	"log"
	"net"
	"os"

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
	opts.DestIp = "224.0.1.129"    // TODO: 224.0.0.107 for pdelays
	opts.Mac = "01:1b:19:00:00:00" // TODO: 01:80:c2:00:00:0e for pdelays
	opts.RecordPackets = true      // Always true for now

	getopt.FlagLong(&opts.Domain, "domain", 'd', "PTP domain")
	getopt.FlagLong(&opts.Iface, "iface", 'i', "Interface to operate on")
	getopt.FlagLong(&opts.IngressLatency, "ilat", 0, "Ingress latency")
	getopt.FlagLong(&opts.EgressLatency, "elat", 0, "Egress latency")
	getopt.FlagLong(&opts.Udp, "", '4', "Use UDP instead of L2")
	getopt.FlagLong(&opts.Ip, "sip", 0, "Source IP for UDP mode")
	getopt.FlagLong(&opts.DestIp, "dip", 0, "Destination IP for UDP mode")
	getopt.FlagLong(&opts.Mac, "mac", 'm', "Destination MAC for L2 mode")
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
