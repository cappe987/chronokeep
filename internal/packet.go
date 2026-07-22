// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package internal

import (
	"encoding/binary"
	"fmt"
	"time"

	ptp "github.com/cappe987/facebook-time/ptp/protocol"
)

type PacketData struct {
	IsTx     bool
	Packet   ptp.Packet
	HwTstamp time.Time
	SwTstamp time.Time
	Iface    string
}

func (pd *PacketData) Print() {
	msgtype := pd.Packet.MessageType()
	hdr := pd.GetHeader()
	rx_ns := pd.HwTstamp.UnixNano() % 1000000000
	rx_s := pd.HwTstamp.Unix()
	seq := hdr.SequenceID
	corr := hdr.CorrectionField.Duration()
	domain := hdr.DomainNumber
	iface := fmt.Sprintf("%6s", pd.Iface)

	var dir string
	if pd.IsTx {
		dir = fmt.Sprintf("%-10s ->", msgtype)
	} else {
		dir = fmt.Sprintf("<- %10s", msgtype)
	}
	fmt.Printf("%s | %s | Seq %d | Dom %d | hwts %d.%09d | Corr %d\n", iface, dir, seq, domain, rx_s, rx_ns, corr)
}

func (pd *PacketData) NormalizeSwtstamp(base time.Time) time.Duration {
	return pd.SwTstamp.Sub(base)
}

func (pd *PacketData) IsSync() bool {
	return pd.Packet.MessageType() == ptp.MessageSync
}

func (pd *PacketData) IsFollowUp() bool {
	return pd.Packet.MessageType() == ptp.MessageFollowUp
}

func (pd *PacketData) IsDelayReq() bool {
	return pd.Packet.MessageType() == ptp.MessageDelayReq
}

func (pd *PacketData) IsDelayResp() bool {
	return pd.Packet.MessageType() == ptp.MessageDelayResp
}

func (pd *PacketData) IsPDelayReq() bool {
	return pd.Packet.MessageType() == ptp.MessagePDelayReq
}

func (pd *PacketData) IsPDelayResp() bool {
	return pd.Packet.MessageType() == ptp.MessagePDelayResp
}

func (pd *PacketData) IsPDelayRespFollowUp() bool {
	return pd.Packet.MessageType() == ptp.MessagePDelayRespFollowUp
}

func (pd *PacketData) IsPDelay() bool {
	msgtype := pd.Packet.MessageType()
	return msgtype == ptp.MessagePDelayReq || msgtype == ptp.MessagePDelayResp || msgtype == ptp.MessagePDelayRespFollowUp
}

func (pd *PacketData) IsInformationPacket() bool {
	msgtype := pd.Packet.MessageType()
	switch msgtype {
	case ptp.MessageAnnounce, ptp.MessageSignaling, ptp.MessageManagement:
		return true
	default:
		return false
	}
}

func (pd *PacketData) ShouldTxTimestampPacket() bool {
	msgtype := pd.Packet.MessageType()
	switch msgtype {
	case ptp.MessageSync, ptp.MessagePDelayResp:
		if pd.IsTwostepFlagSet() {
			return true
		} else {
			return false
		}
	case ptp.MessageDelayReq, ptp.MessagePDelayReq:
		return true
	default:
		return false
	}
}

func (pd *PacketData) GetSequenceID() uint16 {
	hdr := pd.GetHeader()
	return hdr.SequenceID
}

func (pd *PacketData) IsTwostepFlagSet() bool {
	hdr := pd.GetHeader()
	return (hdr.FlagField & ptp.FlagTwoStep) != 0
}

func (pd *PacketData) GetCorrectionField() time.Duration {
	hdr := pd.GetHeader()
	return hdr.CorrectionField.Duration()
}

func (pd *PacketData) GetSyncOriginTimestamp() ptp.Timestamp {
	pkt := pd.Packet.(*ptp.SyncDelayReq)
	return pkt.OriginTimestamp
}

func (pd *PacketData) GetFupOriginTimestamp() ptp.Timestamp {
	pkt := pd.Packet.(*ptp.FollowUp)
	return pkt.PreciseOriginTimestamp
}

func (pd *PacketData) GetDelayRespOriginTimestamp() ptp.Timestamp {
	pkt := pd.Packet.(*ptp.DelayResp)
	return pkt.ReceiveTimestamp
}

func (pd *PacketData) GetPDelayRespRequestReceiptTimestamp() ptp.Timestamp {
	pkt := pd.Packet.(*ptp.PDelayResp)
	return pkt.RequestReceiptTimestamp
}

func (pd *PacketData) GetPDelayRespFupResponseOriginTimestamp() ptp.Timestamp {
	pkt := pd.Packet.(*ptp.PDelayRespFollowUp)
	return pkt.ResponseOriginTimestamp
}

func (pd *PacketData) PidEquals(pid ptp.PortIdentity) bool {
	hdr := pd.GetHeader()
	return hdr.SourcePortIdentity == pid
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
func (port *Port) BuildSync(seq uint16, corr int64) *PacketData {
	twostep := !port.opts.Onestep
	syncHdr := port.buildHeader(ptp.MessageSync, seq, twostep)
	syncHdr.CorrectionField = ptp.Correction(corr << 16)
	// syncHdr.LogMessageInterval = 0

	syncPkt := ptp.SyncDelayReq{
		Header:           syncHdr,
		SyncDelayReqBody: ptp.SyncDelayReqBody{},
	}

	sync := PacketData{
		Packet: &syncPkt,
		IsTx:   true,
	}
	return &sync
}

// Build blank FollowUp
func (port *Port) BuildFollowUp(seq uint16) (*PacketData, error) {

	return nil, fmt.Errorf("Not implemented")
}

func (port *Port) BuildPDelayReq(seq uint16) *PacketData {

	reqHdr := port.buildHeader(ptp.MessagePDelayReq, seq, false)
	reqHdr.LogMessageInterval = 127

	reqPkt := ptp.PDelayReq{
		Header:        reqHdr,
		PDelayReqBody: ptp.PDelayReqBody{},
	}

	req := PacketData{
		Packet: &reqPkt,
		IsTx:   true,
	}
	return &req
}

func (port *Port) BuildDelayReq(seq uint16) *PacketData {

	reqHdr := port.buildHeader(ptp.MessageDelayReq, seq, false)
	reqHdr.LogMessageInterval = 0

	reqPkt := ptp.SyncDelayReq{
		Header:           reqHdr,
		SyncDelayReqBody: ptp.SyncDelayReqBody{},
	}

	req := PacketData{
		Packet: &reqPkt,
		IsTx:   true,
	}
	return &req
}

// Build GM Announce
func (port *Port) BuildAnnounce(seq uint16) *PacketData {
	annoHdr := port.buildHeader(ptp.MessageAnnounce, seq, false)

	// Only set PTP timescale if using Hwtstamp. With SW this results in
	// being off by current UTC offset.
	if !port.opts.SwTstamp {
		annoHdr.FlagField = ptp.FlagPTPTimescale | ptp.FlagCurrentUtcOffsetValid
	}

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
	respHdr.LogMessageInterval = 0
	respHdr.CorrectionField = reqHdr.CorrectionField

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
	respHdr.LogMessageInterval = 127
	respHdr.CorrectionField = reqHdr.CorrectionField

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
func (port *Port) MakeFollowUp(sync *PacketData) *PacketData {
	// TODO: Verify domain first?
	// msgtype := sync.Packet.MessageType()
	// if msgtype != ptp.MessageSync {
	// 	return nil, fmt.Errorf("Expected Sync. Got %s\n", msgtype)

	// }
	syncPkt := sync.Packet.(*ptp.SyncDelayReq)
	syncHdr := syncPkt.Header
	fupHdr := port.buildHeader(ptp.MessageFollowUp, syncHdr.SequenceID, false)
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

	return &fup
}

// Make a PDelayRespFollowUp for a PDelayResp
func (port *Port) MakeFollowUpPDelay(pdelayResp *PacketData) *PacketData {
	respHdr := pdelayResp.GetHeader()
	respPkt := pdelayResp.Packet.(*ptp.PDelayResp)
	fupHdr := port.buildHeader(ptp.MessagePDelayRespFollowUp, respHdr.SequenceID, false)
	fupHdr.LogMessageInterval = 127

	fupPkt := ptp.PDelayRespFollowUp{
		Header: fupHdr,
		PDelayRespFollowUpBody: ptp.PDelayRespFollowUpBody{
			ResponseOriginTimestamp: ptp.NewTimestamp(pdelayResp.HwTstamp),
			RequestingPortIdentity:  respPkt.PDelayRespBody.RequestingPortIdentity,
		},
	}

	fup := PacketData{
		Packet: &fupPkt,
		IsTx:   true,
	}
	return &fup
}
