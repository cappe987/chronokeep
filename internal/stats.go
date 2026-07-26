// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package internal

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	ptp "github.com/cappe987/facebook-time/ptp/protocol"
)

func (port *Port) GetTxtstamps() []time.Time {
	ls := make([]time.Time, len(port.txRecord))
	for i, pd := range port.txRecord {
		ls[i] = pd.HwTstamp
	}
	return ls
}

type syncFup struct {
	sync *PacketData
	fup  *PacketData
}

func (port *Port) GetMeanT1() int64 {
	// TODO: Handle seqid wraparound
	m := make(map[uint16]syncFup)
	for _, pd := range port.rxRecord {
		seq := pd.GetSequenceID()
		switch pd.Packet.MessageType() {
		case ptp.MessageSync:
			syfup := m[seq]
			syfup.sync = &pd
			m[seq] = syfup
		case ptp.MessageFollowUp:
			syfup := m[seq]
			syfup.fup = &pd
			m[seq] = syfup
		default:
		}
	}
	// TODO: Rolling average to not overflow
	total := int64(0)
	count := int64(0)
	for seq, syfup := range m {
		if syfup.sync == nil || syfup.fup == nil {
			fmt.Printf("Skipping seqid %d\n", seq)
			continue
		}
		count += 1
		t2 := syfup.fup.GetFupOriginTimestamp().Time().UnixNano()
		t1 := syfup.sync.HwTstamp.UnixNano()
		total += t2 - t1
	}

	return total / count
}

func (port *Port) GetMeanT4() int64 {
	// TODO: Handle seqid wraparound
	m := make(map[uint16]syncFup)
	for _, pd := range port.txRecord {
		seq := pd.GetSequenceID()
		switch pd.Packet.MessageType() {
		case ptp.MessageDelayReq:
			syfup := m[seq]
			syfup.sync = &pd
			m[seq] = syfup
		default:
		}
	}
	for _, pd := range port.rxRecord {
		seq := pd.GetSequenceID()
		switch pd.Packet.MessageType() {
		case ptp.MessageDelayResp:
			syfup := m[seq]
			syfup.fup = &pd
			m[seq] = syfup
		default:
		}
	}
	// TODO: Rolling average to not overflow
	total := int64(0)
	count := int64(0)
	for seq, syfup := range m {
		if syfup.sync == nil || syfup.fup == nil {
			fmt.Printf("Skipping seqid %d\n", seq)
			continue
		}
		count += 1
		t2 := syfup.fup.GetDelayRespOriginTimestamp().Time().UnixNano()
		t1 := syfup.sync.HwTstamp.UnixNano()
		total += t2 - t1
	}

	return total / count
}

// TODO: Optimize finding groupings
type group struct {
	p1 *PacketData
	p2 *PacketData
	p3 *PacketData
}

// Verify packet is within 5 seconds. Should prevent issues with wraparound of seqid
func timeIsNear(grp *group, p2 *PacketData) bool {
	var p1 *PacketData
	if grp.p1 != nil {
		p1 = grp.p1
	} else if grp.p2 != nil {
		p1 = grp.p2
	} else {
		p1 = grp.p3
	}
	hw1 := p1.HwTstamp
	hw2 := p2.HwTstamp
	start := hw1.Add(time.Second * time.Duration(-5))
	end := hw1.Add(time.Second * time.Duration(5))
	return hw2.After(start) && hw2.Before(end)
}

func getGroupPrimary(grp *group) ptp.MessageType {
	if grp.p1 != nil {
		if grp.p1.IsSync() {
			return ptp.MessageSync
		} else if grp.p1.IsDelayReq() {
			return ptp.MessageDelayReq
		} else if grp.p1.IsPDelayReq() {
			return ptp.MessagePDelayReq
		}
	} else if grp.p2 != nil {
		if grp.p2.IsFollowUp() {
			return ptp.MessageSync
		} else if grp.p2.IsDelayResp() {
			return ptp.MessageDelayReq
		} else if grp.p2.IsPDelayResp() {
			return ptp.MessagePDelayReq
		}
	} else if grp.p3 != nil {
		if grp.p3.IsPDelayRespFollowUp() {
			return ptp.MessagePDelayReq
		}
	}
	panic("Invalid group type")
}

func getGroupSeqId(grp *group) uint16 {
	if grp.p1 != nil {
		return grp.p1.GetSequenceID()
	} else if grp.p2 != nil {
		return grp.p2.GetSequenceID()
	} else if grp.p3 != nil {
		return grp.p3.GetSequenceID()
	}
	panic("Group without packets when looking for seqid")
}

func makeGroup(pd *PacketData) group {
	switch pd.Packet.MessageType() {
	case ptp.MessageSync, ptp.MessageDelayReq, ptp.MessagePDelayReq:
		return group{p1: pd, p2: nil, p3: nil}
	case ptp.MessageFollowUp, ptp.MessageDelayResp:
		return group{p1: nil, p2: pd, p3: nil}
	case ptp.MessagePDelayRespFollowUp:
		return group{p1: nil, p2: nil, p3: pd}
	}
	panic("Unable to create group")
}

func getPdPrimary(pd *PacketData) ptp.MessageType {
	switch pd.Packet.MessageType() {
	case ptp.MessageSync, ptp.MessageFollowUp:
		return ptp.MessageSync
	case ptp.MessageDelayReq, ptp.MessageDelayResp:
		return ptp.MessageDelayReq
	case ptp.MessagePDelayReq, ptp.MessagePDelayResp, ptp.MessagePDelayRespFollowUp:
		return ptp.MessagePDelayReq
	}
	panic("Unable to get primary")
}

func printGroup(grp *group) {
	fmt.Printf("--------\nSeqID %d\n", getGroupSeqId(grp))
	if grp.p1 != nil {
		fmt.Printf("%v\n", grp.p1.Packet.MessageType())
	}
	if grp.p2 != nil {
		fmt.Printf("%v\n", grp.p2.Packet.MessageType())
	}
	if grp.p3 != nil {
		fmt.Printf("%v\n", grp.p3.Packet.MessageType())
	}
}

func addToGroup(grp *group, pd *PacketData) {
	switch getGroupPrimary(grp) {
	case ptp.MessageSync:
		switch pd.Packet.MessageType() {
		case ptp.MessageSync:
			grp.p1 = pd
		case ptp.MessageFollowUp:
			grp.p2 = pd
		}
	case ptp.MessageDelayReq:
		switch pd.Packet.MessageType() {
		case ptp.MessageDelayReq:
			grp.p1 = pd
		case ptp.MessageDelayResp:
			grp.p2 = pd
		}
	case ptp.MessagePDelayReq:
		switch pd.Packet.MessageType() {
		case ptp.MessagePDelayReq:
			grp.p1 = pd
		case ptp.MessagePDelayResp:
			grp.p2 = pd
		case ptp.MessagePDelayRespFollowUp:
			grp.p3 = pd
		}
	}
}

func findGrouping(groups []*group, pd *PacketData) bool {
	seq := pd.GetSequenceID()
	for _, grp := range groups {
		if grp == nil {
			break
		}
		if getPdPrimary(pd) == getGroupPrimary(grp) && seq == getGroupSeqId(grp) && timeIsNear(grp, pd) {
			addToGroup(grp, pd)
			return true
		}
	}
	return false
}

func calcSyfup(sync *PacketData, fup *PacketData) *time.Duration {
	// Calculate t1-t2
	if sync == nil {
		return nil
	}
	if !sync.IsTwostepFlagSet() {
		t1 := sync.GetSyncOriginTimestamp().Time()
		t2 := sync.HwTstamp
		c1 := sync.GetCorrectionField()
		curr_t1 := t1.Sub(t2) + c1
		return &curr_t1
	}
	if fup == nil {
		return nil

	}
	if sync.GetSequenceID() != fup.GetSequenceID() {
		return nil
	}
	t1 := fup.GetFupOriginTimestamp().Time()
	t2 := sync.HwTstamp
	c1 := sync.GetCorrectionField()
	c2 := fup.GetCorrectionField()
	curr_t1 := t1.Sub(t2) + c2 + c1
	// fmt.Printf("t1 %d | t2 %d | c1 %d | c2 %d\n", t1.UnixNano(), t2.UnixNano(), c1, c2)
	return &curr_t1
}

func CalcDelay(req *PacketData, resp *PacketData) *time.Duration {
	// Calculate t4-t3
	if req == nil || resp == nil {
		return nil
	}
	if req.GetSequenceID() != resp.GetSequenceID() {
		return nil
	}
	t4 := resp.GetDelayRespOriginTimestamp().Time()
	t3 := req.HwTstamp
	c4 := resp.GetCorrectionField()
	c3 := req.GetCorrectionField()
	curr_t4 := t4.Sub(t3) - c4 - c3
	// fmt.Printf("T4: %d\n", curr_t4)
	return &curr_t4
}

type PacketStat struct {
	msgtype ptp.MessageType
	time    time.Duration
	value   time.Duration
	latency time.Duration
	Twoway  *time.Duration
	// // serverTs        int64
	// // clientTs        int64
	// // correctionField int64
	p1 *PacketData
	p2 *PacketData
	p3 *PacketData
}

func (ps *PacketStat) Value() time.Duration {
	return ps.value
}

func (ps *PacketStat) ToString() string {
	s := int(ps.time.Seconds())
	ms := ps.time.Milliseconds() % 1000
	return fmt.Sprintf("%d.%03d %d", s, ms, ps.value)
}

func (ps *PacketStat) TimeToString() string {
	s := int(ps.time.Seconds())
	ms := ps.time.Milliseconds() % 1000
	return fmt.Sprintf("%d.%03d", s, ms)
}

type MaxMinMean struct {
	Max  time.Duration
	Min  time.Duration
	Mean time.Duration
}

type Stats struct {
	Syncs      []PacketStat
	Delays     []PacketStat
	Twoways    []PacketStat
	FwdAcc     []PacketStat
	T1M        MaxMinMean
	T4M        MaxMinMean
	PDelayM    MaxMinMean
	TwowayM    MaxMinMean
	FwdAccM    MaxMinMean
	T1LatM     MaxMinMean
	T4LatM     MaxMinMean
	PDelayLatM MaxMinMean
}

type WebChart struct {
	Title  string
	CssId  string
	Labels string
	Values string
	Max    int64
	Min    int64
	Mean   int64
	Color  string
}

type WebStats struct {
	Peertopeer         bool
	T1Chart            WebChart
	T4Chart            WebChart
	PDelayChart        WebChart
	TwowayChart        WebChart
	FwdAccChart        WebChart
	SyncLatencyChart   WebChart
	DelayLatencyChart  WebChart
	PDelayLatencyChart WebChart
}

func (stats *Stats) AddSync(sync, fup *PacketData, t1 time.Duration, normTs time.Duration) {
	latency := GetSyncLatency(sync, fup)
	ps := PacketStat{msgtype: ptp.MessageSync, p1: sync, p2: fup, time: normTs, value: t1, latency: latency}
	stats.Syncs = append(stats.Syncs, ps)
}
func (stats *Stats) AddDelay(req, resp *PacketData, t4 time.Duration, normTs time.Duration) {
	latency := GetDelayLatency(req, resp)
	ps := PacketStat{msgtype: ptp.MessageDelayReq, p1: req, p2: resp, time: normTs, value: t4, latency: latency}
	stats.Delays = append(stats.Delays, ps)
}

func (stats *Stats) AddPDelay(req, resp, respFup *PacketData, pdelay time.Duration, normTs time.Duration) {
	latency := GetPDelayLatency(req, resp)
	ps := PacketStat{msgtype: ptp.MessageDelayReq, p1: req, p2: resp, p3: respFup, time: normTs, value: pdelay, latency: latency}
	stats.Delays = append(stats.Delays, ps)
}

func (stats *Stats) AddTwoway(twoway time.Duration, normTs time.Duration) {
	ps := PacketStat{time: normTs, value: twoway}
	stats.Twoways = append(stats.Twoways, ps)
}

func (stats *Stats) AddFwdAcc(fwdAcc time.Duration, normTs time.Duration) {
	ps := PacketStat{time: normTs, value: fwdAcc}
	stats.FwdAcc = append(stats.FwdAcc, ps)
}

func GetSyncLatency(sync *PacketData, fup *PacketData) time.Duration {
	if sync.IsTwostepFlagSet() {
		return sync.HwTstamp.Sub(fup.GetFupOriginTimestamp().Time())
	}
	return sync.HwTstamp.Sub(sync.GetSyncOriginTimestamp().Time())
}

func GetDelayLatency(req *PacketData, resp *PacketData) time.Duration {
	return resp.GetDelayRespOriginTimestamp().Time().Sub(req.HwTstamp)
}

func GetPDelayLatency(req, resp *PacketData) time.Duration {
	return resp.HwTstamp.Sub(req.HwTstamp)
}

func outputValues(f *os.File, header string, list []PacketStat) {
	_, _ = f.WriteString(header)
	for _, ps := range list {
		_, _ = f.WriteString(fmt.Sprintf("%s\n", ps.ToString()))
	}
}

func outputLatency(f *os.File, header string, list []PacketStat) {
	_, _ = f.WriteString(header)
	for _, ps := range list {
		_, _ = f.WriteString(fmt.Sprintf("%s %d\n", ps.TimeToString(), ps.latency))
	}
}

func (stats *Stats) GenerateFile(peertopeer bool, filename string) {
	if filename == "" {
		return
	}
	f, _ := os.Create(filename)
	defer f.Close()
	if peertopeer {
		outputValues(f, "SYNC_TIME_ERROR\n", stats.Syncs)
		outputValues(f, "\nMEASURED_PDELAY\n", stats.Delays)
		outputValues(f, "\nFWD_ACCURACY\n", stats.FwdAcc)
		outputLatency(f, "\nSYNC_LATENCY\n", stats.Syncs)
		outputLatency(f, "\nPDELAY_LATENCY\n", stats.Delays)
	} else {
		outputValues(f, "SYNC_TIME_ERROR\n", stats.Syncs)
		outputValues(f, "\nDELAY_TIME_ERROR\n", stats.Delays)
		outputValues(f, "\nTWOWAY_TIME_ERROR\n", stats.Twoways)
		outputLatency(f, "\nSYNC_LATENCY\n", stats.Syncs)
		outputLatency(f, "\nDELAY_LATENCY\n", stats.Delays)
	}
}

func (port *Port) GetE2eTE() Stats {
	var pkts []PacketData
	pkts = append(port.rxRecord, port.txRecord...)
	var stats Stats

	// Sort on SwTstamp since that's the order we processed them in
	slices.SortFunc(pkts, func(a, b PacketData) int {
		if a.SwTstamp.Before(b.SwTstamp) {
			return -1
		} else {
			return 1
		}
	})

	curr_delay := time.Duration(0)
	curr_t1 := time.Duration(0)
	curr_t4 := time.Duration(0)
	var last_sync *PacketData = nil
	var last_fup *PacketData = nil
	var last_delay_req *PacketData = nil
	var last_delay_resp *PacketData = nil
	total := time.Duration(0)
	total_t1 := time.Duration(0)
	total_t4 := time.Duration(0)
	count := int64(0)
	count_t1 := int64(0)
	count_t4 := int64(0)

	if len(pkts) == 0 {
		return stats
	}

	baseTs := pkts[0].SwTstamp

	// TODO: Use iterative mean calculation to avoid potential overflow
	// TODO: Break out to groupings first? And then do calculations
	for _, pd := range pkts {
		// pd.Print()
		if pd.IsInformationPacket() {
			continue
		}
		// pd.Print()
		normTs := pd.NormalizeSwtstamp(baseTs)
		switch pd.Packet.MessageType() {
		case ptp.MessageSync:
			last_sync = &pd
			ct1 := calcSyfup(last_sync, last_fup)
			if ct1 == nil {
				continue
			}
			curr_t1 = *ct1
			total_t1 += curr_t1
			count_t1 += 1
			// stats.AddT1TE(curr_t1, GetSyncLatency(last_sync, last_fup), normTs)
			stats.AddSync(last_sync, last_fup, curr_t1, normTs)

		case ptp.MessageFollowUp:
			last_fup = &pd
			ct1 := calcSyfup(last_sync, last_fup)
			if ct1 == nil {
				continue
			}
			curr_t1 = *ct1
			total_t1 += curr_t1
			count_t1 += 1
			stats.AddSync(last_sync, last_fup, curr_t1, normTs)
			// stats.AddT1TE(curr_t1, GetSyncLatency(last_sync, last_fup), normTs)
		case ptp.MessageDelayReq:
			last_delay_req = &pd
			ct4 := CalcDelay(last_delay_req, last_delay_resp)
			if ct4 == nil {
				continue
			}
			curr_t4 = *ct4
			curr_delay = (curr_t1 + curr_t4) / 2
			// fmt.Printf("T4: %d | 2Way %d\n", curr_t4, curr_delay)
			total_t4 += curr_t4
			count_t4 += 1
			total += curr_delay
			count += 1
			stats.AddDelay(last_delay_req, last_delay_resp, curr_t4, normTs)
			stats.AddTwoway(curr_delay, normTs)
			// stats.AddT4TE(curr_t4, GetDelayLatency(last_delay_req, last_delay_resp), normTs)
			// stats.AddTwowayTE(curr_delay, normTs)
		case ptp.MessageDelayResp:
			last_delay_resp = &pd
			ct4 := CalcDelay(last_delay_req, last_delay_resp)
			if ct4 == nil {
				continue
			}
			curr_t4 = *ct4
			curr_delay = (curr_t1 + curr_t4) / 2
			// fmt.Printf("T4: %d | 2Way %d\n", curr_t4, curr_delay)
			total_t4 += curr_t4
			count_t4 += 1
			total += curr_delay
			count += 1
			stats.AddDelay(last_delay_req, last_delay_resp, curr_t4, normTs)
			stats.AddTwoway(curr_delay, normTs)
			// stats.AddT4TE(curr_t4, GetDelayLatency(last_delay_req, last_delay_resp), normTs)
			// stats.AddTwowayTE(curr_delay, normTs)
		}

	}

	stats.calcE2eValues()
	return stats
}

func CalcPDelay(req *PacketData, resp *PacketData, respFup *PacketData) *time.Duration {
	if req == nil || resp == nil {
		return nil
	}

	reqSeqId := req.GetSequenceID()
	if reqSeqId != resp.GetSequenceID() {
		return nil
	}

	if resp.IsTwostepFlagSet() && (respFup == nil || reqSeqId != respFup.GetSequenceID()) {
		return nil
	}

	t1 := req.HwTstamp
	t2 := resp.GetPDelayRespRequestReceiptTimestamp().Time()
	t3 := time.Time{}
	t4 := resp.HwTstamp
	c1 := resp.GetCorrectionField()
	c2 := time.Duration(0)
	if resp.IsTwostepFlagSet() {
		t3 = respFup.GetPDelayRespFupResponseOriginTimestamp().Time()
		c2 = respFup.GetCorrectionField()
	}
	// fmt.Printf("t1 %d | t2 %d | t3 %d | t4 %d | c1 %d | c2 %d\n",
	// 	t1.UnixNano(), t2.UnixNano(), t3.UnixNano(), t4.UnixNano(), c1, c2)
	pdelay := ((t4.Sub(t1)) - (t3.Sub(t2)) - c1 - c2) / 2
	return &pdelay
}

func (port *Port) GetP2pTE() Stats {
	var pkts []PacketData
	pkts = append(port.rxRecord, port.txRecord...)
	var stats Stats

	// Sort on SwTstamp since that's the order we processed them in
	slices.SortFunc(pkts, func(a, b PacketData) int {
		if a.SwTstamp.Before(b.SwTstamp) {
			return -1
		} else {
			return 1
		}
	})

	var filtered []PacketData
	for _, pd := range pkts {
		if pd.IsPDelayReq() && !pd.PidEquals(port.portIdentity) {
			continue
		}
		if (pd.IsPDelayResp() || pd.IsPDelayRespFollowUp()) && pd.PidEquals(port.portIdentity) {
			continue
		}
		filtered = append(filtered, pd)
	}

	curr_t1 := time.Duration(0)
	curr_pdelay := time.Duration(0)
	var last_sync *PacketData = nil
	var last_fup *PacketData = nil
	var last_pdelay_req *PacketData = nil
	var last_pdelay_resp *PacketData = nil
	var last_pdelay_resp_fup *PacketData = nil
	total_t1 := time.Duration(0)
	total_fwd_acc := time.Duration(0)
	total_pdelay := time.Duration(0)
	count_t1 := int64(0)
	count_fwd_acc := int64(0)
	count_pdelay := int64(0)

	if len(pkts) == 0 {
		return stats
	}

	baseTs := pkts[0].SwTstamp

	// TODO: Use iterative mean calculation to avoid potential overflow
	for _, pd := range filtered {
		// pd.Print()
		if pd.IsInformationPacket() {
			continue
		}
		normTs := pd.NormalizeSwtstamp(baseTs)
		switch pd.Packet.MessageType() {
		case ptp.MessageSync:
			last_sync = &pd
			ct1 := calcSyfup(last_sync, last_fup)
			if ct1 == nil {
				continue
			}
			curr_t1 = *ct1
			total_t1 += curr_t1
			count_t1 += 1
			// Need at least one pdelay before we start measuring
			stats.AddSync(last_sync, last_fup, curr_t1, normTs)
			if count_pdelay > 0 {
				stats.AddFwdAcc(curr_t1+curr_pdelay, normTs)
				count_fwd_acc += 1
				total_fwd_acc += curr_t1 + curr_pdelay
			}
		case ptp.MessageFollowUp:
			last_fup = &pd
			ct1 := calcSyfup(last_sync, last_fup)
			if ct1 == nil {
				continue
			}
			curr_t1 = *ct1
			total_t1 += curr_t1
			count_t1 += 1
			stats.AddSync(last_sync, last_fup, curr_t1, normTs)
			if count_pdelay > 0 {
				stats.AddFwdAcc(curr_t1+curr_pdelay, normTs)
				count_fwd_acc += 1
				total_fwd_acc += curr_t1 + curr_pdelay
			}
		case ptp.MessagePDelayReq:
			last_pdelay_req = &pd
			pdelay := CalcPDelay(last_pdelay_req, last_pdelay_resp, last_pdelay_resp_fup)
			if pdelay == nil {
				continue
			}
			curr_pdelay = *pdelay
			total_pdelay += *pdelay
			count_pdelay += 1
			stats.AddPDelay(last_pdelay_req, last_pdelay_resp, last_pdelay_resp_fup, curr_pdelay, normTs)
		case ptp.MessagePDelayResp:
			last_pdelay_resp = &pd
			pdelay := CalcPDelay(last_pdelay_req, last_pdelay_resp, last_pdelay_resp_fup)
			if pdelay == nil {
				continue
			}
			curr_pdelay = *pdelay
			total_pdelay += *pdelay
			count_pdelay += 1
			stats.AddPDelay(last_pdelay_req, last_pdelay_resp, last_pdelay_resp_fup, curr_pdelay, normTs)
		case ptp.MessagePDelayRespFollowUp:
			last_pdelay_resp_fup = &pd
			pdelay := CalcPDelay(last_pdelay_req, last_pdelay_resp, last_pdelay_resp_fup)
			if pdelay == nil {
				continue
			}
			curr_pdelay = *pdelay
			total_pdelay += *pdelay
			count_pdelay += 1
			stats.AddPDelay(last_pdelay_req, last_pdelay_resp, last_pdelay_resp_fup, curr_pdelay, normTs)
		}
	}

	stats.calcP2pValues()
	return stats
}

func calcMaxMinMean(list []PacketStat) (time.Duration, time.Duration, time.Duration) {
	min := time.Duration(math.MaxInt64)
	max := time.Duration(math.MinInt64)
	avg := time.Duration(0)
	for i, ps := range list {
		avg += (ps.value - avg) / (time.Duration(i) + 1)
		if ps.value < min {
			min = ps.value
		}
		if ps.value > max {
			max = ps.value
		}
	}
	return max, min, avg
}

func calcMaxMinMeanLat(list []PacketStat) (time.Duration, time.Duration, time.Duration) {
	min := time.Duration(math.MaxInt64)
	max := time.Duration(math.MinInt64)
	avg := time.Duration(0)
	for i, ps := range list {
		avg += (ps.latency - avg) / (time.Duration(i) + 1)
		if ps.latency < min {
			min = ps.latency
		}
		if ps.value > max {
			max = ps.latency
		}
	}
	return max, min, avg
}

func (stats *Stats) calcMaxMinMeanT1() {
	max, min, mean := calcMaxMinMean(stats.Syncs)
	stats.T1M = MaxMinMean{Max: max, Min: min, Mean: mean}
}

func (stats *Stats) calcMaxMinMeanT4() {
	max, min, mean := calcMaxMinMean(stats.Delays)
	stats.T4M = MaxMinMean{Max: max, Min: min, Mean: mean}
}

func (stats *Stats) calcMaxMinMeanTwoway() {
	max, min, mean := calcMaxMinMean(stats.Twoways)
	stats.TwowayM = MaxMinMean{Max: max, Min: min, Mean: mean}
}

func (stats *Stats) calcMaxMinMeanPDelay() {
	max, min, mean := calcMaxMinMean(stats.Delays)
	stats.PDelayM = MaxMinMean{Max: max, Min: min, Mean: mean}
}

func (stats *Stats) calcMaxMinMeanFwdAcc() {
	max, min, mean := calcMaxMinMean(stats.FwdAcc)
	stats.FwdAccM = MaxMinMean{Max: max, Min: min, Mean: mean}
}

func (stats *Stats) calcMaxMinMeanT1Lat() {
	max, min, mean := calcMaxMinMeanLat(stats.Syncs)
	stats.T1LatM = MaxMinMean{Max: max, Min: min, Mean: mean}
}

func (stats *Stats) calcMaxMinMeanT4Lat() {
	max, min, mean := calcMaxMinMeanLat(stats.Delays)
	stats.T4LatM = MaxMinMean{Max: max, Min: min, Mean: mean}
}

func (stats *Stats) calcMaxMinMeanPDelayLat() {
	max, min, mean := calcMaxMinMeanLat(stats.Delays)
	stats.PDelayLatM = MaxMinMean{Max: max, Min: min, Mean: mean}
}

func (stats *Stats) calcE2eValues() {
	stats.calcMaxMinMeanT1()
	stats.calcMaxMinMeanT4()
	stats.calcMaxMinMeanTwoway()
	stats.calcMaxMinMeanT1Lat()
	stats.calcMaxMinMeanT4Lat()
}

func (stats *Stats) calcP2pValues() {
	stats.calcMaxMinMeanT1()
	stats.calcMaxMinMeanPDelay()
	stats.calcMaxMinMeanFwdAcc()
	stats.calcMaxMinMeanT1Lat()
	stats.calcMaxMinMeanPDelayLat()
}

// func prepareWebChartLat(title string, mmm MaxMinMean, list []PacketStat) WebChart {
// 	labels := "["
// 	values := "["
// 	for _, ps := range list {
// 		s := int(ps.time.Seconds())
// 		ms := ps.time.Milliseconds() % 1000
// 		if i == 0 {
// 			labels = fmt.Sprintf("%s%d.%03d\n", labels, s, ms)
// 			values = fmt.Sprintf("%s%d\n", values, ps.latency)

// 		} else {
// 			labels = fmt.Sprintf("%s, %d.%03d\n", labels, s, ms)
// 			values = fmt.Sprintf("%s, %d\n", values, ps.latency)
// 		}
// 	}
// 	labels += "]"
// 	values += "]"
// 	id := strings.Replace(strings.ToLower(title), " ", "-", -1)
// 	return WebChart{
// 		Title:  title,
// 		CssId:  id,
// 		Labels: labels,
// 		Values: values,
// 		Max:    mmm.Max,
// 		Min:    mmm.Min,
// 		Mean:   mmm.Mean,
// 	}
// }

func getVal(ps PacketStat) time.Duration {
	return ps.value
}
func getLat(ps PacketStat) time.Duration {
	return ps.latency
}

func prepareWebChart(title string, mmm MaxMinMean, list []PacketStat, getF func(PacketStat) time.Duration, color string) WebChart {
	labels := "["
	values := "["
	for i, ps := range list {
		s := int(ps.time.Seconds())
		ms := ps.time.Milliseconds() % 1000
		if i == 0 {
			labels = fmt.Sprintf("%s%d.%03d", labels, s, ms)
			values = fmt.Sprintf("%s%d", values, getF(ps))
		} else {
			labels = fmt.Sprintf("%s, %d.%03d", labels, s, ms)
			values = fmt.Sprintf("%s, %d", values, getF(ps))
		}
	}
	labels += "]"
	values += "]"
	id := strings.Replace(strings.ToLower(title), " ", "-", -1)
	return WebChart{
		Title:  title,
		CssId:  id,
		Labels: labels,
		Values: values,
		Max:    mmm.Max.Nanoseconds(),
		Min:    mmm.Min.Nanoseconds(),
		Mean:   mmm.Mean.Nanoseconds(),
		Color:  color,
	}
}

const chartBlue = "#36a2eb"
const chartOrange = "#ff9f40"

func (stats *Stats) GetWebStats(p2p bool) WebStats {
	return WebStats{
		Peertopeer:  p2p,
		T1Chart:     prepareWebChart("T1 TE", stats.T1M, stats.Syncs, getVal, chartBlue),
		T4Chart:     prepareWebChart("T4 TE", stats.T4M, stats.Delays, getVal, chartBlue),
		PDelayChart: prepareWebChart("PDelay TE", stats.PDelayM, stats.Delays, getVal, chartBlue),
		TwowayChart: prepareWebChart("Twoway TE", stats.TwowayM, stats.Twoways, getVal, chartBlue),
		FwdAccChart: prepareWebChart("FwdAcc TE", stats.FwdAccM, stats.FwdAcc, getVal, chartBlue),
		// Note: Sync/DelayLatency not applicable to BC
		SyncLatencyChart:   prepareWebChart("T1 Latency", stats.T1LatM, stats.Syncs, getLat, chartOrange),
		DelayLatencyChart:  prepareWebChart("T4 Latency", stats.T4LatM, stats.Delays, getLat, chartOrange),
		PDelayLatencyChart: prepareWebChart("PDelay Turnaround Latency", stats.PDelayLatM, stats.Delays, getLat, chartOrange),
	}
}
