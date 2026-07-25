package internal

import (
	"testing"
	"time"

	ptp "github.com/cappe987/facebook-time/ptp/protocol"
)

func init() {
	InitTesting()
}

func setCorrectionField(hdr *ptp.Header, corr int64) {
	hdr.CorrectionField = ptp.Correction(corr << 16)
}

func TestPDelayCalc(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()
	const baseMicro = 10000 * 1000000
	const expectNs = 650

	pdelayReq := Client.BuildPDelayReq(0)
	setCorrectionField(pdelayReq.GetHeader(), 400)
	pdelayReq.HwTstamp = time.UnixMicro(baseMicro + 1) // RX tstamp (t2)
	pdelayResp := Server.MakeResponsePDelay(pdelayReq)
	pdelayResp.HwTstamp = time.UnixMicro(baseMicro + 2) // TX tstamp (t3)
	pdelayRespFup := Server.MakeFollowUpPDelay(pdelayResp)
	setCorrectionField(pdelayRespFup.GetHeader(), 300)

	// Adjust timestamps to true value. We fake it above so resp are created correctly
	pdelayReq.HwTstamp = time.UnixMicro(baseMicro)      // TX tstamp (t1)
	pdelayResp.HwTstamp = time.UnixMicro(baseMicro + 3) // RX stamp (t4)

	ns := CalcPDelay(pdelayReq, pdelayResp, pdelayRespFup)
	if ns == nil {
		t.Fatalf("Calculation resulted in nil")
	}
	if *ns != expectNs {
		t.Errorf("Expected pdelay %d. Got %d", expectNs, *ns)
	}
}
