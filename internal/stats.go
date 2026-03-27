package internal

import (
	"fmt"
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
