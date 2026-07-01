module intime

go 1.23.4

replace github.com/facebook/time => /home/casan/hub/time

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/facebook/time v0.0.0-20260205203828-80f2c810b480
	github.com/pborman/getopt/v2 v2.1.0
	golang.org/x/net v0.38.0
	golang.org/x/sys v0.31.0
)

require github.com/coder/websocket v1.8.15 // indirect
