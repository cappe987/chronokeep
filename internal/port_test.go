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
	txp = Port{IfaceStr: p1}
	rxp = Port{IfaceStr: p2}
	txp.Silent = true
	rxp.Silent = true
}

func TestSinglePacket(t *testing.T) {
	txp.Init(app, 0)
	rxp.Init(app, 0)
	defer txp.Deinit()
	defer rxp.Deinit()
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

	rxpd, err := rxp.ReceiveOne(true)
	if err != nil {
		t.Fatalf("Error receiving packet: %s", err)
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
		t.Errorf("Wrong seqID received. Expected %d. Got %d", sync.GetHeader().DomainNumber, rxpd.GetHeader().DomainNumber)
	}
}
