package internal

import (
	"testing"
	"time"
)

const (
	p1 = "veth1"
	p2 = "veth2"
)

var opts = CommonOpts{Mode: "pkt"}
var app *App
var txp Port
var rxp Port

func init() {
	opts.InitDefaults()
	opts.SwTstamp = true
	app = NewApp(opts, false, true)
}

func initPorts() {
	txp = Port{IfaceStr: p1}
	rxp = Port{IfaceStr: p2}
	txp.Silent = true
	rxp.Silent = true
	txp.Init(app, 0)
	rxp.Init(app, 0)
}

func deinitPorts() {
	txp.Deinit()
	rxp.Deinit()
}

func TestSinglePacket(t *testing.T) {
	initPorts()
	defer deinitPorts()
	const seq = 99
	const corr = 123
	sync := txp.BuildSync(seq, corr)
	txpd, err := txp.Transmit(sync)
	if err != nil {
		t.Fatalf("Error sending sync: %s", err)
	}

	if !txpd.IsTx {
		t.Errorf("Transmitted sync does not have IsTx set")
	}
	if !txpd.IsTwostepFlagSet() {
		t.Errorf("Transmitted sync does not have TwoStepFlag set")
	}

	rxpd, err := rxp.ReceiveOneEvent()
	if err != nil {
		t.Fatalf("Error receiving sync: %s", err)
	}
	if rxpd.IsTx {
		t.Errorf("Received sync has IsTx set")
	}
	if rxpd.GetSequenceID() != seq {
		t.Errorf("Wrong seqID received. Expected %d. Got %d", seq, rxpd.GetSequenceID())
	}
	if rxpd.GetCorrectionField() != time.Duration(corr) {
		t.Errorf("Wrong seqID received. Expected %d. Got %d", corr, rxpd.GetCorrectionField())
	}
	if rxpd.GetHeader().DomainNumber != sync.GetHeader().DomainNumber {
		t.Errorf("Wrong domain received. Expected %d. Got %d", sync.GetHeader().DomainNumber, rxpd.GetHeader().DomainNumber)
	}
}
func TestSyncFup(t *testing.T) {
	initPorts()
	defer deinitPorts()

	txSync, txFup := txp.TransmitSyncFup()
	rxSync, err := rxp.ReceiveOneEvent()
	if err != nil {
		t.Fatalf("Error receiving sync: %s", err)
	}
	rxFup, err := rxp.ReceiveOneGeneral()
	if err != nil {
		t.Fatalf("Error receiving fup: %s", err)
	}

	txSyncTs := txSync.HwTstamp
	txFupTs := txFup.GetFupOriginTimestamp().Time()
	rxSyncTs := rxSync.HwTstamp
	rxFupTs := rxFup.GetFupOriginTimestamp().Time()

	if txSyncTs != txFupTs {
		t.Errorf("Expected TX Fup to contain tstamp matching TX Sync")
	}
	if txSyncTs != rxFupTs {
		t.Errorf("Expected RX Fup to contain tstamp matching TX Sync")
	}
	if rxSyncTs.IsZero() {
		t.Errorf("Expected RX Sync to have RX tstamp")
	}
	if rxFup.HwTstamp.IsZero() {
		t.Errorf("Expected RX Fup to no have RX tstamp")
	}
}

func TestManyPackets(t *testing.T) {
	initPorts()
	defer deinitPorts()
	// Too high number here fills up the socket queue. 222 is the limit for me.
	const count = 100

	for _ = range count {
		txp.TransmitAnnounce()
	}

	var pkts []PacketData
	for i := range count {
		pd, err := rxp.ReceiveOneGeneral()
		if err != nil {
			t.Fatalf("Error receiving packet id %d: %s", i, err)
		}
		pkts = append(pkts, *pd)
		if pd.GetSequenceID() != uint16(i) {
			t.Errorf("Wrong seqID received. Expected %d. Got %d", i, pd.GetSequenceID())
		}
	}
	if len(pkts) != count {
		t.Errorf("Expected %d packets. Got %d", count, len(pkts))
	}

	for i, pd := range txp.txRecord {
		if pd.GetSequenceID() != uint16(i) {
			t.Errorf("Wrong seqID in TX record. Expected %d. Got %d", i, pd.GetSequenceID())
		}
	}

	for i, pd := range rxp.rxRecord {
		if pd.GetSequenceID() != uint16(i) {
			t.Errorf("Wrong seqID in RX record. Expected %d. Got %d", i, pd.GetSequenceID())
		}
	}
}

func TestDelayReply(t *testing.T) {
	initPorts()
	defer deinitPorts()
	txp.TransmitDelayReq()
	rxReq, err := rxp.ReceiveOneEvent()
	if err != nil {
		t.Fatalf("Error receiving delayReq: %s", err)
	}
	txResp := rxp.ReplyToDelayReq(rxReq)
	if txResp == nil {
		t.Fatalf("Error sending delayResp")
	}

	rxResp, err := txp.ReceiveOneGeneral()
	if err != nil {
		t.Fatalf("Error receiving delayResp: %s", err)
	}

	rxReqTs := rxReq.HwTstamp
	rxRespTs := rxResp.GetDelayRespOriginTimestamp().Time()
	if rxReqTs != rxRespTs && !rxReqTs.IsZero() {
		t.Errorf("Expected RX Req tstamp to match Resp OriginTimestamp")
	}
}

func TestPDelayReply(t *testing.T) {
	initPorts()
	defer deinitPorts()
	txReq := txp.TransmitPDelayReq()
	rxReq, err := rxp.ReceiveOneEvent()
	if err != nil {
		t.Fatalf("Error receiving delayReq: %s", err)
	}
	txResp, txRespFup := rxp.ReplyToPDelayReq(rxReq)
	if txResp == nil || txRespFup == nil {
		t.Fatalf("Error sending pdelayResp/pdelayRespFup")
	}

	rxResp, err := txp.ReceiveOneEvent()
	if err != nil {
		t.Fatalf("Error receiving pdelayResp: %s", err)
	}
	rxRespFup, err := txp.ReceiveOneGeneral()
	if err != nil {
		t.Fatalf("Error receiving pdelayRespFup: %s", err)
	}

	t1 := txReq.HwTstamp
	t2 := rxResp.GetPDelayRespRequestReceiptTimestamp().Time()
	t3 := rxRespFup.GetPDelayRespFupResponseOriginTimestamp().Time()
	t4 := rxResp.HwTstamp
	if t1.IsZero() {
		t.Errorf("Expected TX Req tstamp")
	}
	if t2.IsZero() {
		t.Errorf("Expected RX Req tstamp")
	}
	if t3.IsZero() {
		t.Errorf("Expected TX Resp tstamp")
	}
	if t4.IsZero() {
		t.Errorf("Expected RX Resp tstamp")
	}
}
