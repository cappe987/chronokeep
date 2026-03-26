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
		dir = fmt.Sprintf("%s ->", msgtype)
	} else {
		dir = fmt.Sprintf("<- %s", msgtype)
	}
	fmt.Printf("%s | Seq %d | Dom %d | hwts %d.%09d | Corr %d\n", dir, seq, domain, rx_s, rx_ns, corr)
}

func (pd *PacketData) GetHeader() *ptp.Header {
	msgtype := pd.Packet.MessageType()
	switch msgtype {
	case ptp.MessageSync, ptp.MessageDelayReq:
		pkt := pd.Packet.(*ptp.SyncDelayReq)
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

func (port *Port) BuildPacket(msgtype ptp.MessageType, seq uint16) (*PacketData, error) {
	hdr := ptp.Header{
		SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0),
		Version:            ptp.Version,
		MessageLength:      uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})), //#nosec G115
		DomainNumber:       port.opts.Domain,
		FlagField:          ptp.FlagTwoStep, // TODO: check mode and pkt type
		SequenceID:         seq,
		SourcePortIdentity: port.portIdentity,
		LogMessageInterval: 0,
		ControlField:       0,
	}
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

// TODO: More Build functions

// Make a DelayResp response for a DelayReq
func (port *Port) MakeResponseDelay(delayReq *PacketData) (*PacketData, error) {

	return nil, fmt.Errorf("Not implemented")
}

// Make a PDelayResp response for a PDelayRequest
func (port *Port) MakeResponsePDelay(pdelayReq *PacketData) (*PacketData, error) {

	return nil, fmt.Errorf("Not implemented")
}

// Make a FollowUp for a Sync
func (port *Port) MakeFollowUp(sync *PacketData) (*PacketData, error) {

	return nil, fmt.Errorf("Not implemented")
}

// Make a PDelayRespFollowUp for a PDelayResp
func (port *Port) MakeFollowUpPdelay(pdelayResp *PacketData) (*PacketData, error) {

	return nil, fmt.Errorf("Not implemented")
}
