package internal

import (
	"fmt"
	"slices"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
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

func calcSyfup(sync *PacketData, fup *PacketData) int64 {
	// Calculate t1-t2
	t1 := fup.GetFupOriginTimestamp().Nano()
	t2 := sync.HwTstamp.UnixNano()
	c1 := sync.GetCorrectionField()
	c2 := fup.GetCorrectionField()
	curr_t1 := (t1 - t2) + c2 + c1
	// fmt.Printf("T1: %d\n", curr_t1)
	return curr_t1
}

func calcDelay(req *PacketData, resp *PacketData) int64 {
	// Calculate t4-t3
	t4 := resp.GetDelayRespOriginTimestamp().Nano()
	t3 := req.HwTstamp.UnixNano()
	c4 := resp.GetCorrectionField()
	c3 := req.GetCorrectionField()
	curr_t4 := t4 - t3 - c4 - c3
	// fmt.Printf("T4: %d\n", curr_t4)
	return curr_t4
}

func (port *Port) GetMeanTE() (int64, int64, int64) {
	var pkts []PacketData
	pkts = append(port.rxRecord, port.txRecord...)

	// Sort on SwTstamp since that's the order we processed them in
	slices.SortFunc(pkts, func(a, b PacketData) int {
		if a.SwTstamp.Before(b.SwTstamp) {
			return -1
		} else {
			return 1
		}
	})

	curr_delay := int64(0)
	curr_t1 := int64(0)
	curr_t4 := int64(0)
	var last_sync *PacketData = nil
	var last_fup *PacketData = nil
	var last_delay_req *PacketData = nil
	var last_delay_resp *PacketData = nil
	total := int64(0)
	total_t1 := int64(0)
	total_t4 := int64(0)
	count := int64(0)
	count_t1 := int64(0)
	count_t4 := int64(0)

	// TODO: Use iterative mean calculation to avoid potential overflow
	for _, pd := range pkts {
		// pd.Print()
		if pd.IsInformationPacket() {
			continue
		}
		// pd.Print()
		switch pd.Packet.MessageType() {
		case ptp.MessageSync:
			last_sync = &pd
			if last_fup != nil && pd.GetSequenceID() == last_fup.GetSequenceID() {
				curr_t1 = calcSyfup(last_sync, last_fup)
				total_t1 += curr_t1
				count_t1 += 1
			}
		case ptp.MessageFollowUp:
			last_fup = &pd
			if last_sync != nil && pd.GetSequenceID() == last_sync.GetSequenceID() {
				curr_t1 = calcSyfup(last_sync, last_fup)
				total_t1 += curr_t1
				count_t1 += 1
			}
		case ptp.MessageDelayReq:
			last_delay_req = &pd
			if last_delay_resp != nil && pd.GetSequenceID() == last_delay_resp.GetSequenceID() {
				curr_t4 = calcDelay(last_delay_req, last_delay_resp)
				curr_delay = (curr_t1 + curr_t4) / 2
				// fmt.Printf("T4: %d | 2Way %d\n", curr_t4, curr_delay)
				total_t4 += curr_t4
				count_t4 += 1
				total += curr_delay
				count += 1
			}
		case ptp.MessageDelayResp:
			last_delay_resp = &pd
			if last_delay_req != nil && pd.GetSequenceID() == last_delay_req.GetSequenceID() {

				curr_t4 = calcDelay(last_delay_req, last_delay_resp)
				curr_delay = (curr_t1 + curr_t4) / 2
				// fmt.Printf("T4: %d | 2Way %d\n", curr_t4, curr_delay)
				total_t4 += curr_t4
				count_t4 += 1
				total += curr_delay
				count += 1
			}
		}

	}

	return (total_t1 / count_t1), (total_t4 / count_t4), (total / count)
}

func calcPDelay(req *PacketData, resp *PacketData, respFup *PacketData) *int64 {
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

	t1 := req.HwTstamp.UnixNano()
	// t2 := resp.GetPDelayRespRequestReceiptTimestamp().Time().UnixNano()
	// TODO: This seriously needs better handling. An empty field converted
	// to Time() does not correspond to 1970. Golang interprets it as
	// 0001-01-01 00:00:00. Patching facebook/time to add a .Nano() method.
	// Facebook probably uses the Unix time for all calculations. I prefer
	// integers.
	t2 := resp.GetPDelayRespRequestReceiptTimestamp().Nano()
	t3 := int64(0)
	t4 := resp.HwTstamp.UnixNano()
	c1 := resp.GetCorrectionField()
	c2 := int64(0)
	if resp.IsTwostepFlagSet() {
		t3 = respFup.GetPDelayRespFupResponseOriginTimestamp().Nano()
		c2 = respFup.GetCorrectionField()
	}
	// fmt.Printf("Pdelay: t1 %d | t2 %d | t3 %d | t4 %d | c1 %d | c2 %d\n", t1, t2, t3, t4, c1, c2)
	pdelay := ((t4 - t1) - (t3 - t2) - c1 - c2) / 2
	return &pdelay
}

// TODO: Clean this up and return data more structured
func (port *Port) GetP2pTE() (int64, int64, int64) {
	var pkts []PacketData
	pkts = append(port.rxRecord, port.txRecord...)

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

	curr_t1 := int64(0)
	curr_pdelay := int64(0)
	var last_sync *PacketData = nil
	var last_fup *PacketData = nil
	var last_pdelay_req *PacketData = nil
	var last_pdelay_resp *PacketData = nil
	var last_pdelay_resp_fup *PacketData = nil
	total_t1 := int64(0)
	total_fwd_acc := int64(0)
	total_pdelay := int64(0)
	count_t1 := int64(0)
	count_fwd_acc := int64(0)
	count_pdelay := int64(0)

	// TODO: Use iterative mean calculation to avoid potential overflow
	for _, pd := range filtered {
		// pd.Print()
		if pd.IsInformationPacket() {
			continue
		}
		switch pd.Packet.MessageType() {
		case ptp.MessageSync:
			last_sync = &pd
			if last_fup != nil && pd.GetSequenceID() == last_fup.GetSequenceID() {
				curr_t1 = calcSyfup(last_sync, last_fup)
				total_t1 += curr_t1
				count_t1 += 1
				// Need at least one pdelay before we start measuring
				if count_pdelay > 0 {
					count_fwd_acc += 1
					total_fwd_acc += curr_t1 + curr_pdelay
					fmt.Printf("T1 %d | Pdelay %d\n", curr_t1, curr_pdelay)
				}
			}
		case ptp.MessageFollowUp:
			last_fup = &pd
			if last_sync != nil && pd.GetSequenceID() == last_sync.GetSequenceID() {
				curr_t1 = calcSyfup(last_sync, last_fup)
				total_t1 += curr_t1
				count_t1 += 1
				if count_pdelay > 0 {
					count_fwd_acc += 1
					total_fwd_acc += curr_t1 + curr_pdelay
					fmt.Printf("T1 %d | Pdelay %d\n", curr_t1, curr_pdelay)
				}
			}
		case ptp.MessagePDelayReq:
			last_pdelay_req = &pd
			pdelay := calcPDelay(last_pdelay_req, last_pdelay_resp, last_pdelay_resp_fup)
			if pdelay != nil {
				curr_pdelay = *pdelay
				total_pdelay += *pdelay
				count_pdelay += 1
			}
		case ptp.MessagePDelayResp:
			last_pdelay_resp = &pd
			pdelay := calcPDelay(last_pdelay_req, last_pdelay_resp, last_pdelay_resp_fup)
			if pdelay != nil {
				curr_pdelay = *pdelay
				total_pdelay += *pdelay
				count_pdelay += 1
			}
		case ptp.MessagePDelayRespFollowUp:
			last_pdelay_resp_fup = &pd
			pdelay := calcPDelay(last_pdelay_req, last_pdelay_resp, last_pdelay_resp_fup)
			if pdelay != nil {
				curr_pdelay = *pdelay
				total_pdelay += *pdelay
				count_pdelay += 1
			}
		}
	}

	return (total_t1 / count_t1), (total_pdelay / count_pdelay), (total_fwd_acc / count_fwd_acc)
}
