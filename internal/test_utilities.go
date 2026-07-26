package internal

import (
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

func InitTestPorts() {
	AppServer = NewApp(opts, false, true)
	AppClient = NewApp(opts, false, true)
	ResetMockTimestamp()
	Server = Port{
		IfaceStr:       p1name,
		Silent:         true,
		MockTimestamps: true,
		RecordPackets:  false,
	}
	Client = Port{
		IfaceStr:       p2name,
		Silent:         true,
		MockTimestamps: true,
		RecordPackets:  false,
	}
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

func MockTimestamp() time.Time {
	mutex.Lock()
	idx := tsIdx
	tsIdx += 1
	mutex.Unlock()

	// Each timestamp happens 1 microsecond apart
	return time.UnixMicro(baseMicro + (idx * 1))
}
