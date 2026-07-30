// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
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

func GenSyncFup(seq uint16, ts []time.Time, corr int64) []PacketData {
	sync := Server.BuildSync(seq, 0)
	sync.HwTstamp = ts[0]
	sync.IsTx = false
	setCorrectionField(sync.GetHeader(), corr)
	fup := Server.MakeFollowUp(sync)
	fup.IsTx = false
	sync.HwTstamp = ts[1]
	return []PacketData{*sync, *fup}
}

func GenDelayExchange(seq uint16, ts []time.Time, corr int64) []PacketData {
	delayReq := Client.BuildDelayReq(seq)
	setCorrectionField(delayReq.GetHeader(), corr)
	delayReq.HwTstamp = ts[1]
	delayResp := Server.MakeResponseDelay(delayReq)
	delayResp.IsTx = false
	// Adjust timestamps to true value. We fake it above so resp are created correctly
	delayReq.HwTstamp = ts[0]
	return []PacketData{*delayReq, *delayResp}
}

func GenPDelayExchange(seq uint16, ts []time.Time, corr int64) []PacketData {
	pdelayReq := Client.BuildPDelayReq(seq)
	setCorrectionField(pdelayReq.GetHeader(), corr)
	pdelayReq.HwTstamp = ts[1] // RX tstamp (t2)
	pdelayResp := Server.MakeResponsePDelay(pdelayReq)
	pdelayResp.IsTx = false
	pdelayResp.HwTstamp = ts[2] // TX tstamp (t3)
	pdelayRespFup := Server.MakeFollowUpPDelay(pdelayResp)
	pdelayRespFup.IsTx = false
	setCorrectionField(pdelayRespFup.GetHeader(), corr+50)
	// Adjust timestamps to true value. We fake it above so resp are created correctly
	pdelayReq.HwTstamp = ts[0]  // TX tstamp (t1)
	pdelayResp.HwTstamp = ts[3] // RX stamp (t4)
	return []PacketData{*pdelayReq, *pdelayResp, *pdelayRespFup}
}

type stest struct {
	name   string
	expect int64
	got    int64
}

func TestE2eCalc(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()
	var pkts []PacketData

	pkts = append(pkts, GenDelayExchange(0, MockTimestamps(2), 200)...)
	pkts = append(pkts, GenSyncFup(0, MockTimestamps(2), 300)...)
	pkts = append(pkts, GenDelayExchange(1, MockTimestamps(2), 150)...)
	pkts = append(pkts, GenSyncFup(1, MockTimestamps(2), 250)...)
	pkts = append(pkts, GenDelayExchange(2, MockTimestamps(2), 200)...)
	pkts = append(pkts, GenSyncFup(2, MockTimestamps(2), 350)...)
	// txRecord and rxRecord are concatenated when calculating stats
	Client.rxRecord = pkts

	stats := Client.GetE2eTE()
	subtests := []stest{
		{name: "T1 Max", expect: -650, got: stats.T1M.Max.Nanoseconds()},
		{name: "T1 Min", expect: -750, got: stats.T1M.Min.Nanoseconds()},
		{name: "T1 Mean", expect: -700, got: stats.T1M.Mean.Nanoseconds()},
		{name: "T4 Max", expect: 700, got: stats.T4M.Max.Nanoseconds()},
		{name: "T4 Min", expect: 600, got: stats.T4M.Min.Nanoseconds()},
		{name: "T4 Mean", expect: 634, got: stats.T4M.Mean.Nanoseconds()},
		{name: "Twoway Max", expect: 300, got: stats.TwowayM.Max.Nanoseconds()},
		{name: "Twoway Min", expect: -75, got: stats.TwowayM.Min.Nanoseconds()},
		{name: "Twoway Mean", expect: 75, got: stats.TwowayM.Mean.Nanoseconds()},
	}

	for _, st := range subtests {
		if st.expect != st.got {
			t.Errorf("Expected %s to be %d. Got %d", st.name, st.expect, st.got)
		}
	}
}

func TestP2pCalc(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()
	var pkts []PacketData

	pkts = append(pkts, GenPDelayExchange(0, MockTimestamps(4), 200)...)
	pkts = append(pkts, GenSyncFup(0, MockTimestamps(2), 300)...)
	pkts = append(pkts, GenPDelayExchange(1, MockTimestamps(4), 150)...)
	pkts = append(pkts, GenSyncFup(1, MockTimestamps(2), 250)...)
	pkts = append(pkts, GenPDelayExchange(2, MockTimestamps(4), 200)...)
	pkts = append(pkts, GenSyncFup(2, MockTimestamps(2), 350)...)
	// txRecord and rxRecord are concatenated when calculating stats
	Client.rxRecord = pkts

	stats := Client.GetP2pTE()
	subtests := []stest{
		{name: "T1 Max", expect: -650, got: stats.T1M.Max.Nanoseconds()},
		{name: "T1 Min", expect: -750, got: stats.T1M.Min.Nanoseconds()},
		{name: "T1 Mean", expect: -700, got: stats.T1M.Mean.Nanoseconds()},
		{name: "PDelay Max", expect: 825, got: stats.PDelayM.Max.Nanoseconds()},
		{name: "PDelay Min", expect: 775, got: stats.PDelayM.Min.Nanoseconds()},
		{name: "PDelay Mean", expect: 792, got: stats.PDelayM.Mean.Nanoseconds()},
		{name: "FwdAcc Max", expect: 125, got: stats.FwdAccM.Max.Nanoseconds()},
		{name: "FwdAcc Min", expect: 75, got: stats.FwdAccM.Min.Nanoseconds()},
		{name: "FwdAcc Mean", expect: 91, got: stats.FwdAccM.Mean.Nanoseconds()},
	}

	for _, st := range subtests {
		if st.expect != st.got {
			t.Errorf("Expected %s to be %d. Got %d", st.name, st.expect, st.got)
		}
	}
}
