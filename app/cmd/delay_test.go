package cmd

import (
	"bytes"
	. "ckeep/internal"
	"strings"
	"sync"
	"testing"
	"time"
)

func init() {
	InitTesting()
}

func getDelayOpts() DelayOpts {
	var delayOpts = DelayOpts{}
	delayOpts.InitDefaults()
	delayOpts.IntervalTime = time.Duration(10) * time.Millisecond
	delayOpts.Count = 1
	return delayOpts
}

// Close client if too long has passed.
// Maybe the server didn't reply and it got stuck waiting.
func timeoutClient(quit chan int) {
	time.Sleep(time.Duration(200) * time.Millisecond)
	close(quit)
}

func runDelayMode(t *testing.T, delayOpts DelayOpts) string {
	var wg sync.WaitGroup

	var serverOut bytes.Buffer
	var clientOut bytes.Buffer
	AppClient.SetOutput(&clientOut)
	AppServer.SetOutput(&serverOut)
	wg.Go(func() {
		server(AppServer, &delayOpts, &Server)
	})
	go timeoutClient(AppClient.QuitCh)
	time.Sleep(time.Duration(10) * time.Millisecond)
	client(AppClient, &delayOpts, &Client)
	out := clientOut.String()
	AppServer.Quit()
	wg.Wait()
	// Remote trailing newline so we don't get an empty string
	lines := strings.Split(out[:len(out)-1], "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected two lines of output. Got %s", out)
	}
	return lines[1]
}

func TestDelay(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()

	const expectReqTs = "1.000000000"
	const expectRespTs = "1.000001000"
	delayOpts := getDelayOpts()
	line := runDelayMode(t, delayOpts)
	// Output: Seq 0 | ReqTS 10000.000000000 | RespTs 10000.000001000 | Cf 0
	words := strings.Split(line, " ")
	if expectReqTs != words[4] {
		t.Errorf("Expected ReqTS %s. Got %s", expectReqTs, words[6])
	}
	if expectRespTs != words[7] {
		t.Errorf("Expected RespTS %s. Got %s", expectRespTs, words[9])
	}

}

func TestPDelay(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()

	Client.SetTestIngressLatency(150)
	Client.SetTestEgressLatency(125)
	Server.SetTestIngressLatency(250)
	Server.SetTestEgressLatency(195)
	const expectPdelay = "777"
	delayOpts := getDelayOpts()
	delayOpts.Peertopeer = true

	line := runDelayMode(t, delayOpts)
	// Output: PDelay seq 0: 1000
	words := strings.Split(line, " ")
	pdelay := words[len(words)-1]
	if pdelay != expectPdelay {
		t.Errorf("Expected pdelay %s. Got %s", expectPdelay, pdelay)
	}
}
