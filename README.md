# InTime

**TODO:**
- Log levels
- Fix htons in port.go
- Convert to using time.Duration in most places?
- RX filters out packets from self. Problems when two instances run with the
  same hardcoded PortID. Base PortID on MAC.

Web:
- Fix P2P stats
- Indicate when capture starts
- Allow downloading data file for external graph generation
- Better error handling for invalid config
- More modes. Block user from swapping while one is active
- In-web graphs. Display min/max/mean next to or under every graph
- Template dependency graph?

Uses a [fork](https://github.com/cappe987/facebook-time) of
[facebook/time](https://github.com/facebook/time) for onestep support and
.Nano() function for timestamps.

Build InTime

```bash
# Host architecture
go build
# Without web server
go build --tags noweb
# ARM64
GOARCH=arm64 go build -ldflags "-s -w"
# ARMv7
GOARCH=arm go build -ldflags "-s -w"
# ARMv5
GOARCH=arm GOARM=5 go build -ldflags "-s -w"
```

The extra flags are useful on systems with limited resources and minimizes the binary size.
- `-s` strips binary
- `-w` removes DWARF debugging info

