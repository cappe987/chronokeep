package internal

import (
	"fmt"
	"log"
	"net"
	"strings"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/pborman/getopt/v2"
)

type CommonOpts struct {
	OptList           []Opt
	Mode              string
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

type Opt struct {
	Short rune
	Long  string
	Help  string
	Usage string
	Mode  string
}

func (opts *CommonOpts) AddOpt(v interface{}, short rune, long string, usage string, help string) Opt {
	opt := Opt{Short: short, Long: long, Help: help, Usage: usage}
	opts.OptList = append(opts.OptList, opt)
	getopt.FlagLong(v, long, short, help)
	return opt
}

func (opts *CommonOpts) AddModeOpt(mode string, v interface{}, short rune, long string, usage string, help string) Opt {
	opt := Opt{Short: short, Long: long, Help: help, Usage: usage, Mode: mode}
	opts.OptList = append(opts.OptList, opt)
	getopt.FlagLong(v, long, short, help)
	return opt
}

func (opts *CommonOpts) DefineCommonFlags() {
	// TODO: IP multicast not working yet
	opts.DestIp = "224.0.1.129" // TODO: 224.0.0.107 for pdelays
	opts.Mac = "01:1b:19:00:00:00"
	opts.RecordPackets = true

	// opts.AddOpt(&opts.Udp, '4', "udp", "Use IPv4 UDP instead of L2")

	opts.AddOpt(&opts.Udp, '4', "udp", "", "Use IPv4 UDP instead of L2")
	opts.AddOpt(&opts.Domain, 'd', "domain", "<value>", "PTP domain (0-255)")
	opts.AddOpt(&opts.Iface, 'i', "iface", "<iface>", "Interface to operate on")
	opts.AddOpt(&opts.Ip, 0, "sip", "<IP>", "Source IP for UDP mode")
	opts.AddOpt(&opts.DestIp, 0, "dip", "<IP>", "Destination IP for UDP mode")
	opts.AddOpt(&opts.Mac, 'm', "mac", "<MAC>", "Destination MAC for L2 mode. P2P always uses link-local")
	opts.AddOpt(&opts.SwTstamp, 'S', "swtstamp", "", "Software timestamping")
	opts.AddOpt(&opts.Onestep, 'o', "onestep", "", "Onestep timestamping")
	opts.AddOpt(&opts.IngressLatency, 0, "ilat", "<ns>", "Ingress latency (ns)")
	opts.AddOpt(&opts.EgressLatency, 0, "elat", "<ns>", "Egress latency (ns)")
	opts.AddOpt(&opts.Help, 'h', "help", "", "Show this help menu")
}

func (opts *CommonOpts) Usage() {
	hdr := strings.Repeat("-", 16)
	fmt.Printf("%s InTime %s %s\n", hdr, opts.Mode, hdr)
	fmt.Println(opts.getUsage())
}

func (opts *CommonOpts) getUsage() string {
	longest := 0
	for _, opt := range opts.OptList {
		tmp := len(opt.Long+opt.Usage) + 1
		if tmp > longest {
			longest = tmp
		}
	}
	longest += 1

	str := "Common options:\n"
	foundMode := false
	for _, opt := range opts.OptList {
		if opt.Mode != "" && !foundMode {
			foundMode = true
			str += fmt.Sprintf("\n%s mode options\n", strings.Title(opt.Mode))
		}
		var short string
		if opt.Short == 0 {
			short = "   "
		} else {
			short = "-" + string(opt.Short) + ","
		}
		long := opt.Long
		if opt.Usage != "" {
			long = long + "=" + opt.Usage
		}
		long = fmt.Sprintf("%-[2]*[1]s", long, longest)
		str += fmt.Sprintf(" %s --%s %s\n", short, long, opt.Help)

	}
	return str
}

func (opts *CommonOpts) Validate() bool {
	if opts.Help {
		opts.Usage()
		return false
	}

	var err error
	err = nil

	if opts.Udp {
		if opts.Ip == "" {
			err = fmt.Errorf("Must specify source IP with --sip\n")
		} else if opts.Iface == "" {
			iface, err := InterfaceFromIP(opts.Ip)
			if err != nil {
				err = fmt.Errorf("Unable to find interface with IP %s\n", opts.Ip)
			}
			opts.Iface = iface
		}
	} else {
		if opts.Iface == "" {
			err = fmt.Errorf("Must specify interface with --iface")
		}
	}
	if err != nil {
		opts.Usage()
		fmt.Printf("Error: %s\n", err)
		return false
	}
	return true
}

func (opts *CommonOpts) Parse() bool {
	err := getopt.Getopt(nil)
	if err != nil {
		opts.Usage()
		fmt.Printf("Error: %s\n", err)
		return false
	}
	if !opts.Validate() {
		return false
	}
	return true
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
