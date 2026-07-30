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
)

func getTeOpts() TeOpts {
	teOpts, _ := InitTeOpts()
	teOpts.Interval = 10 // milliseconds
	p1 := PortOpts{GM: true}
	p2 := PortOpts{}
	teOpts.Ports["veth1"] = p1
	teOpts.Ports["veth2"] = p2
	return teOpts
}

func init() {
	InitTesting()
}

func runTeModeTest(topts *TeOpts) string {
	var wg sync.WaitGroup
	var out bytes.Buffer
	AppServer.SetOutput(&out)
	wg.Go(func() {
		RunTeMode(topts, AppServer)
	})
	time.Sleep(time.Duration(1) * time.Second)
	AppServer.Quit()
	wg.Wait()
	return out.String()
}

func setupTe(topts *TeOpts) {
	ValidateTeOpts(topts, &AppServer.Opts)
	topts.server.MockTimestamps = true
	topts.client.MockTimestamps = true
	topts.server.Silent = true
	topts.client.Silent = true
}

// Mean T1: -2174
func parseTeOutput(line string) int64 {
	words := strings.Fields(line)
	val, _ := strconv.ParseInt(words[2], 10, 64)
	return val
}

func validateTeMode(t *testing.T, out string) {
	lines := ParseLines(out)
	if len(lines) != 3 {
		t.Errorf("Expected three lines in output. Got %s", out)
	}
	for _, line := range lines {
		val := parseTeOutput(line)
		if val > 5000 || val < -5000 {
			t.Errorf("Sanity check failed. Expected value below +-5000. Got %s", line)
		}
	}
}

func TestTeE2e(t *testing.T) {
	CreateApps()
	topts := getTeOpts()
	setupTe(&topts)
	out := runTeModeTest(&topts)
	validateTeMode(t, out)
}

func TestTeP2p(t *testing.T) {
	CreateApps()
	topts := getTeOpts()
	topts.Peertopeer = true
	setupTe(&topts)
	out := runTeModeTest(&topts)
	validateTeMode(t, out)
}
