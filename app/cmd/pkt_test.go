// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package cmd

import (
	"bytes"
	. "ckeep/internal"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	ptp "github.com/cappe987/facebook-time/ptp/protocol"
)

func init() {
	InitTesting()
}

func getPktOpts() PktOpts {
	var pktOpts = PktOpts{}
	pktOpts.InitDefaults()
	pktOpts.IntervalTime = time.Duration(10) * time.Millisecond
	pktOpts.Count = 1
	return pktOpts
}

func runPktMode(poS, poC *PktOpts) (string, string) {
	var wg sync.WaitGroup
	var outS bytes.Buffer
	var outC bytes.Buffer
	AppClient.SetOutput(&outC)
	AppServer.SetOutput(&outS)
	wg.Go(func() {
		normalMode(AppServer, &Server, poS)
	})
	normalMode(AppClient, &Client, poC)

	AppServer.Quit()
	wg.Wait()
	return outS.String(), outC.String()
}

func TestPktTxOne(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()
	poS := getPktOpts()
	poC := getPktOpts()
	Server.Silent = false
	Client.Silent = false

	out, _ := runPktMode(&poS, &poC)
	lines := ParseLines(out)
	if len(lines) != 1 {
		t.Errorf("Expected one line in output. Got %s", out)
	}
}

func TestPktTxMany(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()
	poS := getPktOpts()
	poC := getPktOpts()
	Server.Silent = false
	Client.Silent = false

	poC.Count = 50
	poS.RxMode = true
	out, _ := runPktMode(&poS, &poC)
	lines := ParseLines(out)
	if len(lines) != 50 {
		t.Errorf("Expected 50 lines in output. Got %s", out)
	}
}

type cliPkt struct {
	port      string
	isIngress bool
	msgtype   ptp.MessageType
	seq       uint16
	dom       uint8
	hwts      string
	corr      string
}

// veth1 | <-       SYNC | Seq 0 | Dom 0 | hwts 1.000002000 | Corr 0
// veth2 | SYNC       -> | Seq 51 | Dom 0 | hwts 1.000101000 | Corr 0
//
//	0       1         2     3  4     5  6   7      8            9   10
func parseCliPacket(line string) cliPkt {
	words := strings.Fields(strings.ReplaceAll(line, "|", ""))
	// fmt.Printf("Words: %v\n", words)
	pkt := cliPkt{port: words[0]}

	if words[1] == "<-" {
		pkt.isIngress = true
		pkt.msgtype = StringToMessageType(words[2])
	} else {
		pkt.isIngress = false
		pkt.msgtype = StringToMessageType(words[1])
	}
	seq, _ := strconv.Atoi(words[4])
	pkt.seq = uint16(seq)
	dom, _ := strconv.Atoi(words[6])
	pkt.dom = uint8(dom)
	pkt.hwts = words[8]
	pkt.corr = words[10]
	return pkt
}

func msgtypeInList(msgtype ptp.MessageType, list []cliPkt) bool {
	for _, pkt := range list {
		if pkt.msgtype == msgtype {
			return true
		}
	}
	return false
}

func TestPktSequence(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()
	poS := getPktOpts()
	poC := getPktOpts()
	Server.Silent = false
	Client.Silent = false

	poS.RxMode = true
	poC.SequenceTypes = []ptp.MessageType{
		ptp.MessageSync,
		ptp.MessageDelayReq,
		ptp.MessagePDelayReq,
		ptp.MessagePDelayResp,
		ptp.MessageFollowUp,
		ptp.MessageDelayResp,
		ptp.MessagePDelayRespFollowUp,
		ptp.MessageAnnounce,
		// Message types not fully supported yet and requires TLVs
		// ptp.MessageSignaling,
		// ptp.MessageManagement,
	}

	outS, outC := runPktMode(&poS, &poC)
	// Note: packets may be processed out-of-order on RX since event and general
	// are independent of each other.
	rxLines := ParseLines(outS)
	if len(rxLines) != len(poC.SequenceTypes) {
		t.Fatalf("Expected %d packets. Got %s", len(poC.SequenceTypes), outS)
	}
	txLines := ParseLines(outC)
	if len(rxLines) != len(poC.SequenceTypes) {
		t.Fatalf("Expected %d packets. Got %s", len(poC.SequenceTypes), outC)
	}

	var sent []cliPkt
	var received []cliPkt
	for _, line := range rxLines {
		pkt := parseCliPacket(line)
		received = append(received, pkt)
	}
	for _, line := range txLines {
		pkt := parseCliPacket(line)
		sent = append(sent, pkt)
	}
	for _, msgtype := range poC.SequenceTypes {
		if !msgtypeInList(msgtype, sent) {
			t.Errorf("Expected message %s to be sent", msgtype.String())
		}
		if !msgtypeInList(msgtype, received) {
			t.Errorf("Expected message %s to be received", msgtype.String())
		}
	}
	// fmt.Printf("TX: %+v\n", sent)
	// fmt.Printf("RX: %+v\n", received)
}
