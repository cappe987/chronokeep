package cmd

import (
	. "intime/internal"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	ptp "github.com/cappe987/facebook-time/ptp/protocol"
)

type gmOpts struct {
	Interval   uint32
	Peertopeer bool
}

func GmMode() {
	var port Port
	var opts = CommonOpts{Mode: "gm"}
	var gmOpts = gmOpts{}
	interval_ms := uint32(1000)

	opts.DefineCommonFlags()
	opts.RecordPackets = false
	opts.AddModeOpt("gm", &gmOpts.Interval, 'I', "interval", "<ms>", "TX packet interval (ms)")
	opts.AddModeOpt("gm", &gmOpts.Peertopeer, 'P', "peertopeer", "", "P2P Mode")

	if !opts.ParseFile(&gmOpts) {
		return
	}
	if !opts.Parse() {
		return
	}

	////////////////////////////////////////////////////

	Interval := time.Duration(interval_ms) * time.Millisecond

	app := NewApp(opts, false, true)
	err := port.Init(app, 0, 1)
	if err != nil {
		log.Fatalf("Failed initializing port: %s", err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	rxCh := make(chan PacketData, 100)
	quit := make(chan int)
	var ticker *time.Ticker
	running := true
	seq := uint16(0)

	ticker = time.NewTicker(Interval)
	gmTxPackets(&port, &seq, gmOpts.Peertopeer)

	go port.RxMode(rxCh, quit)
	for running {
		select {
		case <-sigs:
			quit <- 0
			running = false
		case pd := <-rxCh:
			pd.Print()
			replyToDelay(&port, &pd, gmOpts.Peertopeer)
		case <-ticker.C:
			gmTxPackets(&port, &seq, gmOpts.Peertopeer)
		}
	}
	// TODO: Requires HW to test
	port.Deinit()
}

func replyToDelay(port *Port, pd *PacketData, pdelayMode bool) {
	if !pdelayMode && pd.IsDelayReq() {
		resp := port.MakeResponseDelay(pd)
		port.Transmit(resp)
		resp.Print()
	} else if pdelayMode && pd.IsPDelayReq() {
		resp := port.MakeResponsePDelay(pd)
		port.Transmit(resp)
		resp.Print()
		respFup := port.MakeFollowUpPDelay(resp)
		port.Transmit(respFup)
		respFup.Print()
	}
}

func gmTxPackets(port *Port, seq *uint16, pdelayMode bool) {

	anno := port.BuildAnnounce(*seq)
	port.Transmit(anno)
	anno.Print()

	sync, err := port.BuildPacket(ptp.MessageSync, *seq)
	if err != nil {
		log.Fatalf("Failed building packet: %s", err)
	}
	port.Transmit(sync)
	sync.Print()
	fup := port.MakeFollowUp(sync)
	port.Transmit(fup)
	fup.Print()

	if pdelayMode {
		pdelayReq := port.BuildPDelayReq(*seq)
		port.Transmit(pdelayReq)
		pdelayReq.Print()
	}

	*seq += uint16(1)
}
