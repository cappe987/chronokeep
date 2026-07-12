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

type TeOpts struct {
	Ports    map[string]PortOpts
	Interval uint32
	// Count       uint32
	Peertopeer  bool
	DelayRecord uint32

	// Internal fields
	IntervalTime time.Duration
	server       *Port
	client       *Port
	Stats        Stats
	Capturing    bool
}

func TeMode() {
	teOpts, opts := InitTeOpts()

	opts.DefineCommonFlags()
	opts.AddModeOpt(opts.Mode, &teOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt(opts.Mode, &teOpts.Peertopeer, 'P', "p2p", "", "Use P2P mode")
	// opts.AddModeOpt(mode, &teOpts.Count, 'c', "count", "<num>", "Number of packets to transmit. 0=infinite")
	opts.AddModeOpt(opts.Mode, &teOpts.DelayRecord, 'D', "delay_record", "<seconds>", "Time to wait until recording starts. Default: 0 seconds")

	if !opts.ParseFile(&teOpts) {
		return
	}
	if !opts.Parse() {
		return
	}

	if !ValidateTeOpts(&teOpts, &opts) {
		return
	}

	app := NewApp(opts, false, true)
	RunTeMode(&teOpts, app)
}
func InitTeOpts() (TeOpts, CommonOpts) {
	var teOpts = TeOpts{}
	var opts = CommonOpts{Mode: "te"}
	opts.InitDefaults()
	teOpts.Interval = uint32(1000)
	teOpts.DelayRecord = uint32(0)

	teOpts.Ports = make(map[string]PortOpts)
	return teOpts, opts
}

func ValidateTeOpts(teOpts *TeOpts, opts *CommonOpts) bool {
	ok := opts.Validate()
	if !ok {
		return false
	}

	if len(teOpts.Ports) != 2 {
		fmt.Printf("Error: two ports are required\n")
		return false
	}
	teOpts.IntervalTime = time.Duration(teOpts.Interval) * time.Millisecond
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
		return false
	}
	if gmCount != 1 {
		return false
	}
	// TODO: Validate port tstamp modes. facebook/time has some helpers.
	// Move out the above part to a validation function?

	p1 := teOpts.Ports[p1name]
	p2 := teOpts.Ports[p2name]
	ip1 := net.ParseIP(p1.IP)
	ip2 := net.ParseIP(p2.IP)

	server := &Port{
		IfaceStr: p1name,
		IP:       ip1,
		DestIP:   ip2,
		PortOpts: p1,
	}
	client := &Port{
		IfaceStr: p2name,
		IP:       ip2,
		DestIP:   ip1,
		PortOpts: p2,
	}
	teOpts.server = server
	teOpts.client = client
	return true
}

func RunTeMode(teOpts *TeOpts, app *App) {
	server := teOpts.server
	client := teOpts.client

	if !app.Cli {
		// In Web we only want to capture client. Maybe this should be
		// configurable later.
		server.Silent = true
	}

	// Port1 is always GM
	// TODO: ingress/egress latency should be set via PortOpts and should not have to use CommonOpts
	// app.Opts.IngressLatency = p1.IngressLatency
	// app.Opts.EgressLatency = p1.EgressLatency
	app.Opts.Ip = server.PortOpts.IP
	app.Opts.DestIp = client.PortOpts.IP
	server.Init(app, 0x64, 1)

	// app.Opts.IngressLatency = p2.IngressLatency
	// app.Opts.EgressLatency = p2.EgressLatency
	app.Opts.Ip = teOpts.client.PortOpts.IP
	app.Opts.DestIp = teOpts.server.PortOpts.IP
	client.Init(app, 0x32, 1)

	sigs := make(chan os.Signal)
	if app.Cli {
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	}
	serverQuit := make(chan int)
	clientQuit := make(chan int)
	serverRx := make(chan PacketData, 100)
	clientRx := make(chan PacketData, 100)
	serverTicker := time.NewTicker(teOpts.IntervalTime)
	clientTicker := time.NewTicker(teOpts.IntervalTime)
	running := true

	delayRecordTimer := time.NewTimer(time.Duration(teOpts.DelayRecord) * time.Second)
	if teOpts.DelayRecord != 0 {
		server.RecordPackets = false
		client.RecordPackets = false
	} else {
		delayRecordTimer.Stop()
	}

	// TODO: Bad file descriptor after seqid ~280. Seems to be an UDP problem

	app.Running = true
	go server.RxMode(serverRx, serverQuit)
	go client.RxMode(clientRx, clientQuit)
	for running {
		select {
		case <-sigs:
			serverQuit <- 0
			clientQuit <- 0
			running = false
			signal.Stop(sigs)
			// TODO: Temporary while developing
			// app.Out <- []byte("exit")
		case pd := <-serverRx:
			// fmt.Printf("Server rx\n")
			// pd.Print()
			if !teOpts.Peertopeer && pd.IsDelayReq() {
				server.ReplyToDelayReq(&pd)
			}
			if teOpts.Peertopeer && pd.IsPDelayReq() {
				server.ReplyToPDelayReq(&pd)
			}
		case pd := <-clientRx:
			// fmt.Printf("Client rx\n")
			// client.ShowPacket(pd)
			if teOpts.Peertopeer && pd.IsPDelayReq() {
				client.ReplyToPDelayReq(&pd)
			}
		case <-serverTicker.C:
			// TODO: Add websocket to transmit
			server.TransmitAnnounce()
			server.TransmitSyncFup()
			if teOpts.Peertopeer {
				server.TransmitPDelayReq()
			}
			// XXX: Something is going on with RX timestamps. Dagger bug?
			// Issue seems to be resolved if we run Deinit()
			// properly to disable timestamping.
		case <-clientTicker.C:
			// TODO: Add websocket to transmit
			if teOpts.Peertopeer {
				client.TransmitPDelayReq()
			} else {
				client.TransmitDelayReq()
			}
		case <-delayRecordTimer.C:
			str1 := "========================"
			str2 := " Starting recording "
			str3 := "========================\n"
			fmt.Printf(str1 + str2 + str3)
			server.RecordPackets = true
			client.RecordPackets = true
		case msg := <-app.In:
			str := string(msg)
			if str == "exit" {
				serverQuit <- 0
				clientQuit <- 0
				running = false
				signal.Stop(sigs)
			} else if str == "record" {
				client.EnableRecording()
			}
		}
	}
	server.Deinit()
	client.Deinit()

	fmt.Printf("\n")
	if teOpts.Peertopeer {
		stats := client.GetP2pTE()
		if app.Cli {
			fmt.Printf("Mean T1: %d\n", stats.T1M.Mean)
			fmt.Printf("Mean Pdelay: %d\n", stats.PDelayM.Mean)
			fmt.Printf("Mean FwdAcc: %d\n", stats.FwdAccM.Mean)
		}
		stats.GenerateFile(true, "measurement.dat")
		teOpts.Stats = stats
		if app.Out != nil {
			app.Out <- []byte("exited")
		}
	} else {
		stats := client.GetE2eTE()
		if app.Cli {
			fmt.Printf("Mean T1: %d\n", stats.T1M.Mean)
			fmt.Printf("Mean T4: %d\n", stats.T4M.Mean)
			fmt.Printf("Mean 2Way: %d\n", stats.TwowayM.Mean)
		}

		stats.GenerateFile(false, "measurement.dat")
		teOpts.Stats = stats
		if app.Out != nil {
			app.Out <- []byte("exited")
		}
	}
}
