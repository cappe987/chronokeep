package internal

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

func DeinitTestPorts() {
	Server.Deinit()
	Client.Deinit()
}
