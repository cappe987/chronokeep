package cmd

import (
	"bytes"
	. "ckeep/internal"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func init() {
	InitTesting()
}

func captureOutput(f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	f()
	log.SetOutput(os.Stderr)
	return buf.String()
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
	cquit := make(chan int)
	squit := make(chan int)
	var wg sync.WaitGroup

	wg.Go(func() {
		captureOutput(func() {
			server(AppServer, &delayOpts, &Server, squit)
		})
	})
	go timeoutClient(cquit)
	time.Sleep(time.Duration(10) * time.Millisecond)
	out := captureOutput(func() {
		client(AppClient, &delayOpts, &Client, cquit)
	})
	squit <- 0
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

	const expectReqTs = "10000.000000000"
	const expectRespTs = "10000.000001000"
	delayOpts := getDelayOpts()
	line := runDelayMode(t, delayOpts)
	// Output: 2026/07/25 19:10:36 Seq 0 | ReqTS 10000.000000000 | RespTs 10000.000001000 | Cf 0
	words := strings.Split(line, " ")
	if expectReqTs != words[6] {
		t.Errorf("Expected ReqTS %s. Got %s", expectReqTs, words[6])
	}
	if expectRespTs != words[9] {
		t.Errorf("Expected RespTS %s. Got %s", expectRespTs, words[9])
	}

}

func TestPDelay(t *testing.T) {
	InitTestPorts()
	defer DeinitTestPorts()

	const expectPdelay = "1000"
	delayOpts := getDelayOpts()
	delayOpts.Peertopeer = true

	line := runDelayMode(t, delayOpts)
	// Output: 2026/07/25 19:10:53 PDelay seq 0: 1000
	words := strings.Split(line, " ")
	pdelay := words[len(words)-1]
	if pdelay != expectPdelay {
		t.Errorf("Expected pdelay %s. Got %s", expectPdelay, pdelay)
	}
}
