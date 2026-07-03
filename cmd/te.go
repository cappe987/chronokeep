package cmd

import (
	"fmt"
	. "intime/internal"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
)

func TeMode() {
	// var port Port
	mode := "te"
	var opts = CommonOpts{Mode: mode}
	opts.InitDefaults()
	var teOpts = TeOpts{}
	// teOpts.Count = 1
	teOpts.Interval = uint32(1000)
	teOpts.DelayRecord = uint32(0)

	opts.DefineCommonFlags()
	opts.AddModeOpt(mode, &teOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt(mode, &teOpts.Peertopeer, 'P', "p2p", "", "Use P2P mode")
	// opts.AddModeOpt(mode, &teOpts.Count, 'c', "count", "<num>", "Number of packets to transmit. 0=infinite")
	opts.AddModeOpt(mode, &teOpts.DelayRecord, 'D', "delay_record", "<seconds>", "Time to wait until recording starts. Default: 0 seconds")

	if !opts.ParseFile(&teOpts) {
		return
	}
	if !opts.Parse() {
		return
	}

	fmt.Printf("%v\n", teOpts)

	if len(teOpts.Ports) != 2 {
		fmt.Printf("Error: two ports are required\n")
		return
	}

	app := NewApp(false)
	RunTeMode(opts, teOpts, app)
}

func RunTeMode(opts CommonOpts, teOpts TeOpts, app *App) {

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
	server.Init(opts, 0x64, 1)

	opts.IngressLatency = p2.IngressLatency
	opts.EgressLatency = p2.EgressLatency
	opts.Ip = p2.IP
	opts.DestIp = p1.IP
	opts.Iface = p2name
	client.Init(opts, 0x32, 1)

	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	quit := make(chan int)
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

	go server.RxMode(serverRx, quit)
	go client.RxMode(clientRx, quit)
	for running {
		select {
		case <-sigs:
			quit <- 0
			running = false
			signal.Stop(sigs)
			// TODO: Temporary while developing
			// app.Out <- []byte("exit")
		case pd := <-serverRx:
			// fmt.Printf("Server rx\n")
			pd.Print()
			if !teOpts.Peertopeer && pd.IsDelayReq() {
				server.ReplyToDelayReq(&pd)
			}
			if teOpts.Peertopeer && pd.IsPDelayReq() {
				server.ReplyToPDelayReq(&pd)
			}
		case pd := <-clientRx:
			// fmt.Printf("Client rx\n")
			pd.Print()
			// TODO: Make channel non-blocking
			if app.WsOut != nil {
				app.WsOut <- []byte(buildHtmxPacket(pd))
			}
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
				quit <- 0
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
		_, _, _, stats := client.GetP2pTE()
		fmt.Printf("Mean T1: %d\n", stats.CalcMeanT1())
		fmt.Printf("Mean Pdelay: %d\n", stats.CalcMeanPDelay())
		fmt.Printf("Mean FwdAcc: %d\n", stats.CalcMeanFwdAcc())
		stats.GenerateFile(true, "measurement.dat")
		if app.Out != nil {
			app.Out <- []byte(buildHtmxStats(teOpts, stats))
		}
	} else {
		_, _, _, stats := client.GetMeanTE()
		fmt.Printf("Mean T1: %d\n", stats.CalcMeanT1())
		fmt.Printf("Mean T4: %d\n", stats.CalcMeanT4())
		fmt.Printf("Mean 2Way: %d\n", stats.CalcMeanTwoway())

		stats.GenerateFile(false, "measurement.dat")
		if app.Out != nil {
			app.Out <- []byte(buildHtmxStats(teOpts, stats))
		}
	}
}

func buildHtmxStats(teOpts TeOpts, stats Stats) string {
	if teOpts.Peertopeer {
		return fmt.Sprintf(`
<p>Mean T1: %d</p>
<p>Mean Pdelay: %d</p>
<p>Mean FwdAcc: %d</p>
`, stats.CalcMeanT1(), stats.CalcMeanPDelay(), stats.CalcMeanFwdAcc())
	} else {
		return fmt.Sprintf(`
<p>Mean T1: %d</p>
<p>Mean T4: %d</p>
<p>Mean 2Way: %d</p>
`, stats.CalcMeanT1(), stats.CalcMeanT4(), stats.CalcMeanTwoway())

	}
}

func buildHtmxPacket(pd PacketData) string {
	msgtype := pd.Packet.MessageType()
	hdr := pd.GetHeader()
	rx_ns := pd.HwTstamp.UnixNano() % 1000000000
	rx_s := pd.HwTstamp.Unix()
	seq := hdr.SequenceID
	corr := hdr.CorrectionField.Duration()
	domain := hdr.DomainNumber
	iface := fmt.Sprintf("%6s", pd.Iface)

	var originTs ptp.Timestamp
	ots_s := int64(0)
	ots_ns := int64(0)
	switch msgtype {
	case ptp.MessageSync:
		originTs = pd.GetSyncOriginTimestamp()
		ots_s = originTs.Nano() / 1000000000
		ots_ns = originTs.Nano() % 1000000000
	case ptp.MessageFollowUp:
		originTs = pd.GetFupOriginTimestamp()
		ots_s = originTs.Nano() / 1000000000
		ots_ns = originTs.Nano() % 1000000000
	case ptp.MessageDelayResp:
		originTs = pd.GetDelayRespOriginTimestamp()
		ots_s = originTs.Nano() / 1000000000
		ots_ns = originTs.Nano() % 1000000000
	case ptp.MessagePDelayResp:
		originTs = pd.GetPDelayRespRequestReceiptTimestamp()
		ots_s = originTs.Nano() / 1000000000
		ots_ns = originTs.Nano() % 1000000000
	case ptp.MessagePDelayRespFollowUp:
		originTs = pd.GetPDelayRespFupResponseOriginTimestamp()
		ots_s = originTs.Nano() / 1000000000
		ots_ns = originTs.Nano() % 1000000000
	}

	return fmt.Sprintf(`
<tbody hx-swap-oob="beforeend:#msgs">
<tr>
    <td>%s</td>
    <td>RX</td>
    <td>%s</td>
    <td>%d</td>
    <td>%d</td>
    <td>%d</td>
    <td>%d.%09d</td>
    <td>%d.%09d</td>
</tr>
</tbody>
`, iface, msgtype, domain, seq, corr, ots_s, ots_ns, rx_s, rx_ns)
}
