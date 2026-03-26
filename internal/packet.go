package internal

import (
	"encoding/binary"
	"fmt"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
)

type PacketData struct {
	IsTx     bool
	Packet   ptp.Packet
	HwTstamp time.Time
	SwTstamp time.Time
}

func (pd *PacketData) Print() {
	msgtype := pd.Packet.MessageType()
	hdr := pd.GetHeader()
	rx_ns := pd.HwTstamp.UnixNano() % 1000000000
	rx_s := pd.HwTstamp.Unix()
	seq := hdr.SequenceID
	corr := hdr.CorrectionField.Duration()
	domain := hdr.DomainNumber
	var dir string
	if pd.IsTx {
		dir = fmt.Sprintf("%-10s ->", msgtype)
	} else {
		dir = fmt.Sprintf("<- %10s", msgtype)
	}
	fmt.Printf("%s | Seq %d | Dom %d | hwts %d.%09d | Corr %d\n", dir, seq, domain, rx_s, rx_ns, corr)
}

func (pd *PacketData) IsSync() bool {
	return pd.Packet.MessageType() == ptp.MessageSync
}

func (pd *PacketData) IsDelayReq() bool {
	return pd.Packet.MessageType() == ptp.MessageDelayReq
}

func (pd *PacketData) IsPDelayReq() bool {
	return pd.Packet.MessageType() == ptp.MessagePDelayReq
}

func (pd *PacketData) IsPDelay() bool {
	msgtype := pd.Packet.MessageType()
	return msgtype == ptp.MessagePDelayReq || msgtype == ptp.MessagePDelayResp || msgtype == ptp.MessagePDelayRespFollowUp
}

func (pd *PacketData) GetHeader() *ptp.Header {
	msgtype := pd.Packet.MessageType()
	switch msgtype {
	case ptp.MessageSync, ptp.MessageDelayReq:
		pkt := pd.Packet.(*ptp.SyncDelayReq)
		return &pkt.Header
	case ptp.MessageDelayResp:
		pkt := pd.Packet.(*ptp.DelayResp)
		return &pkt.Header
	case ptp.MessagePDelayReq:
		pkt := pd.Packet.(*ptp.PDelayReq)
		return &pkt.Header
	case ptp.MessagePDelayResp:
		pkt := pd.Packet.(*ptp.PDelayResp)
		return &pkt.Header
	case ptp.MessageFollowUp:
		pkt := pd.Packet.(*ptp.FollowUp)
		return &pkt.Header
	case ptp.MessagePDelayRespFollowUp:
		pkt := pd.Packet.(*ptp.PDelayRespFollowUp)
		return &pkt.Header
	case ptp.MessageAnnounce:
		pkt := pd.Packet.(*ptp.Announce)
		return &pkt.Header
	case ptp.MessageSignaling:
		pkt := pd.Packet.(*ptp.Signaling)
		return &pkt.Header
	case ptp.MessageManagement:
		pkt := pd.Packet.(*ptp.Management)
		return &pkt.Header
	default:
		fmt.Printf("Invalid message type\n")
		return nil
	}
}

func getSize(msgtype ptp.MessageType) uint16 {
	var size int
	switch msgtype {
	case ptp.MessageSync, ptp.MessageDelayReq:
		size = binary.Size(ptp.SyncDelayReqBody{})
	case ptp.MessageDelayResp:
		size = binary.Size(ptp.DelayRespBody{})
	case ptp.MessagePDelayReq:
		size = binary.Size(ptp.PDelayReqBody{})
	case ptp.MessagePDelayResp:
		size = binary.Size(ptp.PDelayRespBody{})
	case ptp.MessageFollowUp:
		size = binary.Size(ptp.FollowUpBody{})
	case ptp.MessagePDelayRespFollowUp:
		size = binary.Size(ptp.PDelayRespFollowUpBody{})
	case ptp.MessageAnnounce:
		size = binary.Size(ptp.AnnounceBody{})
	// case ptp.MessageSignaling:
	// size = binary.Size(ptp.SignalingBody{})
	// case ptp.MessageManagement:
	// size = binary.Size(ptp.ManagementBody{})
	default:
		fmt.Printf("Invalid message type: %s\n", msgtype)
		size = 0
	}
	return uint16(size)
}

func (port *Port) buildHeader(msgtype ptp.MessageType, seq uint16, setTwoStepFlag bool) ptp.Header {
	twostep := ptp.FlagTwoStep
	if !setTwoStepFlag {
		twostep = 0
	}
	return ptp.Header{
		SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(msgtype, port.opts.TransportSpecific),
		Version:            port.GetVersion(),
		MessageLength:      uint16(binary.Size(ptp.Header{})) + getSize(msgtype), //#nosec G115
		DomainNumber:       port.opts.Domain,
		FlagField:          twostep,
		SequenceID:         seq,
		SourcePortIdentity: port.portIdentity,
		LogMessageInterval: 0,
		ControlField:       0,
	}

}

func (port *Port) BuildPacket(msgtype ptp.MessageType, seq uint16) (*PacketData, error) {
	// hdr := ptp.Header{
	// 	SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageSync, port.opts.TransportSpecific),
	// 	Version:            port.GetVersion(),
	// 	MessageLength:      uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})), //#nosec G115
	// 	DomainNumber:       port.opts.Domain,
	// 	FlagField:          ptp.FlagTwoStep, // TODO: check mode and pkt type
	// 	SequenceID:         seq,
	// 	SourcePortIdentity: port.portIdentity,
	// 	LogMessageInterval: 0,
	// 	ControlField:       0,
	// }
	hdr := port.buildHeader(msgtype, seq, true)
	var p ptp.Packet
	switch msgtype {
	case ptp.MessageSync, ptp.MessageDelayReq:
		p = &ptp.SyncDelayReq{Header: hdr}
	case ptp.MessagePDelayReq:
		p = &ptp.PDelayReq{Header: hdr}
	case ptp.MessagePDelayResp:
		p = &ptp.PDelayResp{Header: hdr}
	case ptp.MessageFollowUp:
		p = &ptp.FollowUp{Header: hdr}
	case ptp.MessageDelayResp:
		p = &ptp.DelayResp{Header: hdr}
	case ptp.MessagePDelayRespFollowUp:
		p = &ptp.PDelayRespFollowUp{Header: hdr}
	case ptp.MessageAnnounce:
		p = &ptp.Announce{Header: hdr}
	case ptp.MessageSignaling:
		p = &ptp.Signaling{Header: hdr}
	// TODO: Handle management packet
	// case ptp.MessageManagement:
	// 	p = &ptp.Management{ManagementMsgHead: ptp.ManagementMsgHead{Header: hdr}, TLV: ptp.ManagementTLV{}}
	default:
		return nil, fmt.Errorf("unsupported type %s", msgtype)
	}

	pd := &PacketData{Packet: p, IsTx: true}
	return pd, nil
}

// Build blank Sync
func (port *Port) BuildSync(seq uint16) (*PacketData, error) {

	return nil, fmt.Errorf("Not implemented")
}

// Build blank FollowUp
func (port *Port) BuildFollowUp(seq uint16) (*PacketData, error) {

	return nil, fmt.Errorf("Not implemented")
}

// Build GM Announce
func (port *Port) BuildAnnounce(seq uint16) *PacketData {
	annoHdr := port.buildHeader(ptp.MessageAnnounce, seq, false)
	annoHdr.FlagField = ptp.FlagPTPTimescale | ptp.FlagCurrentUtcOffsetValid

	annoPkt := ptp.Announce{
		Header: annoHdr,
		AnnounceBody: ptp.AnnounceBody{
			CurrentUTCOffset:     37,
			GrandmasterPriority1: 1,
			TimeSource:           ptp.TimeSourceInternalOscillator,
			GrandmasterClockQuality: ptp.ClockQuality{
				ClockClass:    6,
				ClockAccuracy: 21,
			},
		},
	}

	anno := PacketData{
		Packet: &annoPkt,
		IsTx:   true,
	}
	return &anno
}

// TODO: More Build functions

// Make a DelayResp response for a DelayReq
func (port *Port) MakeResponseDelay(delayReq *PacketData) *PacketData {
	reqHdr := delayReq.GetHeader()
	respHdr := port.buildHeader(ptp.MessageDelayResp, reqHdr.SequenceID, false)

	respPkt := ptp.DelayResp{
		Header: respHdr,
		DelayRespBody: ptp.DelayRespBody{
			ReceiveTimestamp:       ptp.NewTimestamp(delayReq.HwTstamp),
			RequestingPortIdentity: reqHdr.SourcePortIdentity,
		},
	}

	resp := PacketData{
		Packet: &respPkt,
		IsTx:   true,
	}
	return &resp
}

// Make a PDelayResp response for a PDelayRequest
func (port *Port) MakeResponsePDelay(pdelayReq *PacketData) *PacketData {
	reqHdr := pdelayReq.GetHeader()
	respHdr := port.buildHeader(ptp.MessagePDelayResp, reqHdr.SequenceID, true)

	respPkt := ptp.PDelayResp{
		Header: respHdr,
		PDelayRespBody: ptp.PDelayRespBody{
			RequestReceiptTimestamp: ptp.NewTimestamp(pdelayReq.HwTstamp),
			RequestingPortIdentity:  reqHdr.SourcePortIdentity,
		},
	}

	resp := PacketData{
		Packet: &respPkt,
		IsTx:   true,
	}
	return &resp
}

// Make a FollowUp for a Sync
func (port *Port) MakeFollowUp(sync *PacketData) (*PacketData, error) {
	// TODO: Verify domain first?
	msgtype := sync.Packet.MessageType()
	if msgtype != ptp.MessageSync {
		return nil, fmt.Errorf("Expected Sync. Got %s\n", msgtype)

	}
	syncPkt := sync.Packet.(*ptp.SyncDelayReq)
	syncHdr := syncPkt.Header
	fupHdr := ptp.Header{
		SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageFollowUp, port.opts.TransportSpecific),
		Version:            port.GetVersion(),
		MessageLength:      uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.FollowUpBody{})), //#nosec G115
		DomainNumber:       syncHdr.DomainNumber,
		FlagField:          0,
		SequenceID:         syncHdr.SequenceID,
		SourcePortIdentity: port.portIdentity,
		LogMessageInterval: 0,
		ControlField:       0,
	}

	fupPkt := ptp.FollowUp{
		Header: fupHdr,
		FollowUpBody: ptp.FollowUpBody{
			PreciseOriginTimestamp: ptp.NewTimestamp(sync.HwTstamp),
		},
	}

	fup := PacketData{
		Packet: &fupPkt,
		IsTx:   true,
	}

	return &fup, nil
}

// Make a PDelayRespFollowUp for a PDelayResp
func (port *Port) MakeFollowUpPDelay(pdelayResp *PacketData) *PacketData {
	reqHdr := pdelayResp.GetHeader()
	respHdr := port.buildHeader(ptp.MessagePDelayRespFollowUp, reqHdr.SequenceID, false)

	respPkt := ptp.PDelayRespFollowUp{
		Header: respHdr,
		PDelayRespFollowUpBody: ptp.PDelayRespFollowUpBody{
			ResponseOriginTimestamp: ptp.NewTimestamp(pdelayResp.HwTstamp),
			RequestingPortIdentity:  reqHdr.SourcePortIdentity,
		},
	}

	resp := PacketData{
		Packet: &respPkt,
		IsTx:   true,
	}
	return &resp
}
