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

func (port *Port) GetMeanTE() int64 {
	// groups := make([]*group, 0)
	// fmt.Printf("RX\n")
	// for _, pd := range port.rxRecord {
	// 	if pd.IsInformationPacket() {
	// 		continue
	// 	}
	// 	if !findGrouping(groups, &pd) {
	// 		grp := makeGroup(&pd)
	// 		groups = append(groups, &grp)
	// 	}
	// }

	// fmt.Printf("TX\n")
	// for _, pd := range port.txRecord {
	// 	if pd.IsInformationPacket() {
	// 		continue
	// 	}
	// 	if !findGrouping(groups, &pd) {
	// 		grp := makeGroup(&pd)
	// 		groups = append(groups, &grp)
	// 	}
	// }

	// for _, grp := range groups {
	// 	if grp != nil {
	// 		printGroup(grp)
	// 	}

	// }

	var pkts []PacketData
	pkts = append(port.rxRecord, port.txRecord...)

	// fmt.Printf("RXRECORD\n")
	// for _, pd := range port.rxRecord {
	// 	pd.Print()
	// }
	// fmt.Printf("TXRECORD\n")
	// for _, pd := range port.txRecord {
	// 	pd.Print()
	// }

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
	count := int64(0)

	// TODO: Handle correctionField and clean up this shit
	// TODO: Use rolling average?
	for _, pd := range pkts {
		pd.Print()
		if pd.IsInformationPacket() {
			continue
		}
		// pd.Print()
		switch pd.Packet.MessageType() {
		case ptp.MessageSync:
			last_sync = &pd
			if last_fup != nil && pd.GetSequenceID() == last_fup.GetSequenceID() {
				// Calculate t2-t1
				t2 := last_fup.GetFupOriginTimestamp().Time().UnixNano()
				t1 := last_sync.HwTstamp.UnixNano()
				curr_t1 = t2 - t1
				fmt.Printf("T1: %d\n", curr_t1)
			}
		case ptp.MessageFollowUp:
			last_fup = &pd
			if last_sync != nil && pd.GetSequenceID() == last_sync.GetSequenceID() {
				// Calculate t2-t1
				t2 := last_fup.GetFupOriginTimestamp().Time().UnixNano()
				t1 := last_sync.HwTstamp.UnixNano()
				curr_t1 = t2 - t1
				fmt.Printf("T1: %d\n", curr_t1)
			}
		case ptp.MessageDelayReq:
			last_delay_req = &pd
			if last_delay_resp != nil && pd.GetSequenceID() == last_delay_resp.GetSequenceID() {
				// Calculate t2-t1
				t4 := last_delay_resp.GetDelayRespOriginTimestamp().Time().UnixNano()
				t3 := last_delay_req.HwTstamp.UnixNano()
				curr_t4 = t4 - t3
				curr_delay = (curr_t1 + curr_t4) / 2
				fmt.Printf("T4: %d | 2Way %d\n", curr_t4, curr_delay)
				total += curr_delay
				count += 1
			}
		case ptp.MessageDelayResp:
			last_delay_resp = &pd
			if last_delay_req != nil && pd.GetSequenceID() == last_delay_req.GetSequenceID() {
				// Calculate t2-t1
				t4 := last_delay_resp.GetDelayRespOriginTimestamp().Time().UnixNano()
				t3 := last_delay_req.HwTstamp.UnixNano()
				curr_t4 = t4 - t3
				curr_delay = (curr_t1 + curr_t4) / 2
				fmt.Printf("T4: %d | 2Way %d\n", curr_t4, curr_delay)
				total += curr_delay
				count += 1
			}
		}

	}

	fmt.Printf("Total %d | Count %d\n", total, count)

	return total / count
}
