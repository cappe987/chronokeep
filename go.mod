module ckeep

go 1.26.0

// replace github.com/cappe987/facebook-time => /home/casan/hub/time
// replace golang.org/x/sys v0.45.0 => /home/casan/hub/lib/sys

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/cappe987/facebook-time v0.0.0-20260727062102-f7a14d2bc31e
	github.com/facebook/time v0.0.0-20260721164901-c85daed2dfdd
	github.com/pborman/getopt/v2 v2.1.0
	golang.org/x/net v0.57.0
	golang.org/x/sys v0.47.0
)

require github.com/coder/websocket v1.8.15
