package internal

import (
	"testing"
	"time"
)

const (
	p1name = "veth1"
	p2name = "veth2"
)

var opts = CommonOpts{Mode: "pkt"}
var app *App
var txp Port
var rxp Port

func InitTesting() {
	opts.InitDefaults()
	opts.SwTstamp = true
	app = NewApp(opts, false, true)
}

func InitTestPorts() {
	txp = Port{
		IfaceStr:       p1name,
		Silent:         true,
		MockTimestamps: true,
	}
	rxp = Port{
		IfaceStr:       p2name,
		Silent:         true,
		MockTimestamps: true,
	}
	txp.Init(app, 0)
	rxp.Init(app, 0)
}

func DeinitTestPorts() {
	txp.Deinit()
	rxp.Deinit()
}

func TestSinglePacket(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()
	const seq = 99
	const corr = 123
	sync := txp.BuildSync(seq, corr)
	err := txp.Transmit(sync)
	if err != nil {
		t.Fatalf("Error sending sync: %s", err)
	}

	if !sync.IsTx {
		t.Errorf("Transmitted sync does not have IsTx set")
	}
	if !sync.IsTwostepFlagSet() {
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
	InitTestPorts()
	defer DeinitTestPorts()

	txSync, txFup, err := txp.TransmitSyncFup()
	if err != nil {
		t.Fatalf("Error sending sync/fup: %s", err)
	}
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
	InitTestPorts()
	defer DeinitTestPorts()
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
	InitTestPorts()
	defer DeinitTestPorts()
	txp.TransmitDelayReq()
	rxReq, err := rxp.ReceiveOneEvent()
	if err != nil {
		t.Fatalf("Error receiving delayReq: %s", err)
	}
	_, err = rxp.ReplyToDelayReq(rxReq)
	if err != nil {
		t.Fatalf("Error sending delayResp: %s", err)
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
	InitTestPorts()
	defer DeinitTestPorts()
	txReq, err := txp.TransmitPDelayReq()
	if err != nil {
		t.Fatalf("Error sending delayReq: %s", err)
	}
	rxReq, err := rxp.ReceiveOneEvent()
	if err != nil {
		t.Fatalf("Error receiving delayReq: %s", err)
	}
	_, _, err = rxp.ReplyToPDelayReq(rxReq)
	if err != nil {
		t.Fatalf("Error sending pdelayResp/pdelayRespFup: %s", err)
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
