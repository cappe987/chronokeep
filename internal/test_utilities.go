// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package internal

import (
	"strings"
	"sync"
	"time"
)

const (
	p1name = "veth1"
	p2name = "veth2"
)

var opts = CommonOpts{Mode: "pkt"}
var AppServer *App
var AppClient *App
var Server Port
var Client Port

func InitTesting() {
	opts.InitDefaults()
	opts.SwTstamp = true
}

func CreateApps() {
	AppServer = NewApp(opts, false, true)
	AppClient = NewApp(opts, false, true)
	ResetMockTimestamp()
}

func CreateTestPorts() {
	CreateApps()
	Server = Port{
		Name:           p1name,
		Silent:         true,
		MockTimestamps: true,
		RecordPackets:  false,
	}
	Client = Port{
		Name:           p2name,
		Silent:         true,
		MockTimestamps: true,
		RecordPackets:  false,
	}
}

func InitTestPorts() {
	CreateTestPorts()
	Server.Init(AppServer, 0)
	Client.Init(AppClient, 0)
}

func (port *Port) SetTestIngressLatency(lat int64) {
	Client.opts.IngressLatency = lat
}

func (port *Port) SetTestEgressLatency(lat int64) {
	Client.opts.EgressLatency = lat
}

func DeinitTestPorts() {
	Server.Deinit()
	Client.Deinit()
}

// Base time is 1 second after unix
const baseMicro = 1000000
const baseNs = baseMicro * 1000

var tsIdx int64 = 0
var mutex sync.Mutex

func ResetMockTimestamp() {
	mutex.Lock()
	tsIdx = 0
	mutex.Unlock()
}

func mock(idx int64) time.Time {
	// Each timestamp happens 1 microsecond apart
	return time.UnixMicro(baseMicro + (idx * 1))
}

func MockTimestamp() time.Time {
	mutex.Lock()
	idx := tsIdx
	tsIdx += 1
	mutex.Unlock()

	return mock(idx)
}

func MockTimestamps(n int) []time.Time {
	var arr []time.Time
	mutex.Lock()
	for _ = range n {
		arr = append(arr, mock(tsIdx))
		tsIdx += 1
	}
	mutex.Unlock()
	return arr
}

// Close client if too long has passed.
// Maybe the server didn't reply and it got stuck waiting.
func TimeoutApp(quit chan int) {
	time.Sleep(time.Duration(200) * time.Millisecond)
	close(quit)
}

func ParseLines(out string) []string {
	return strings.Split(out[:len(out)-1], "\n")
}
