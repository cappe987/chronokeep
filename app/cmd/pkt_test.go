package cmd

import (
	"bytes"
	. "ckeep/internal"
	"testing"
	"time"
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

func runPktMode(app *App, port *Port, po *PktOpts) string {
	var outBuf bytes.Buffer
	app.SetOutput(&outBuf)
	normalMode(app, port, po)
	return outBuf.String()
}

func TestPktTxOne(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()
	po := getPktOpts()
	Server.Silent = false
	Client.Silent = false

	out := runPktMode(AppServer, &Server, &po)
	lines := ParseLines(out)
	if len(lines) != 1 {
		t.Errorf("Expected one line in output. Got %s", out)
	}
}

func TestPktTxMany(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()
	po := getPktOpts()
	Server.Silent = false
	Client.Silent = false

	po.Count = 50
	out := runPktMode(AppServer, &Server, &po)
	lines := ParseLines(out)
	if len(lines) != 50 {
		t.Errorf("Expected 50 lines in output. Got %s", out)
	}
}
