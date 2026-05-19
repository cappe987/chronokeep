package cmd

import (
	"fmt"
	. "intime/internal"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type portOpts struct {
	IngressLatency uint64
	EgressLatency  uint64
	IP             string
	GM             bool

	// Internal fields
	destIp string
}

type teOpts struct {
	Ports    map[string]portOpts
	Interval uint32
	Count    uint32

	// Internal fields
	intervalTime time.Duration
}

func TeMode() {
	// var port Port
	mode := "te"
	var opts = CommonOpts{Mode: mode}
	var teOpts = teOpts{}
	teOpts.Count = 1
	teOpts.Interval = uint32(1000)
	opts.Iface = "dummy"
	opts.Ip = "dummy"

	opts.DefineCommonFlags()
	opts.AddModeOpt(mode, &teOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt(mode, &teOpts.Count, 'c', "count", "<num>", "Number of packets to transmit. 0=infinite")

	if !opts.ParseFile(&teOpts) {
		return
	}
	if !opts.Parse() {
		return
	}

	teOpts.intervalTime = time.Duration(teOpts.Interval) * time.Millisecond
	fmt.Printf("%v\n", teOpts)

	if len(teOpts.Ports) != 2 {
		fmt.Printf("Error: two ports are required\n")
		return
	}

	// Validate settings
	missingIp := false
	gmCount := 0
	p1name := ""
	p2name := ""
	for name, port := range teOpts.Ports {
		if opts.Udp && port.IP == "" {
			fmt.Printf("Missing IP on port %s\n", name)
			missingIp = true
		}
		if port.GM {
			p1name = name
			gmCount += 1
		} else {
			p2name = name
		}
	}
	if missingIp {
		return
	}
	if gmCount != 1 {
		return
	}

	p1 := teOpts.Ports[p1name]
	p2 := teOpts.Ports[p2name]
	ip1 := net.ParseIP(p1.IP)
	ip2 := net.ParseIP(p2.IP)

	server := Port{
		IfaceStr: p1name,
		IP:       ip1,
		DestIP:   ip2,
	}
	client := Port{
		IfaceStr: p2name,
		IP:       ip2,
		DestIP:   ip1,
	}

	// Port1 is always GM
	opts.IngressLatency = p1.IngressLatency
	opts.EgressLatency = p1.EgressLatency
	opts.Ip = p1.IP
	opts.DestIp = p2.IP
	opts.Iface = p1name
	server.Init(opts, 99, 1)

	opts.IngressLatency = p2.IngressLatency
	opts.EgressLatency = p2.EgressLatency
	opts.Ip = p2.IP
	opts.DestIp = p1.IP
	opts.Iface = p2name
	client.Init(opts, 0, 1)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	quit := make(chan int)
	serverRx := make(chan PacketData, 100)
	clientRx := make(chan PacketData, 100)
	serverTicker := time.NewTicker(teOpts.intervalTime)
	clientTicker := time.NewTicker(teOpts.intervalTime)
	running := true

	// TODO: Bad file descriptor after seqid ~280. Seems to be an UDP problem

	go server.RxMode(serverRx, quit)
	go client.RxMode(clientRx, quit)
	for running {
		select {
		case <-sigs:
			quit <- 0
			running = false
		case pd := <-serverRx:
			// fmt.Printf("Server rx\n")
			pd.Print()
			if pd.IsDelayReq() {
				server.ReplyToDelayReq(&pd)
			}
		case pd := <-clientRx:
			// fmt.Printf("Client rx\n")
			pd.Print()
		case <-serverTicker.C:
			server.TransmitAnnounce()
			server.TransmitSyncFup()
			// TODO: Handle pdelayReq
		case <-clientTicker.C:
			client.TransmitDelayReq()
		}
	}

	fmt.Printf("\n")
	t1, t4, twoway := client.GetMeanTE()
	fmt.Printf("Mean T1: %d\n", t1)
	fmt.Printf("Mean T4: %d\n", t4)
	fmt.Printf("Mean 2Way: %d\n", twoway)

	server.Deinit()
	client.Deinit()
}
