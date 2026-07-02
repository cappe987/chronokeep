# InTime

**TODO:**
- Log levels
- Fix htons in port.go
- Convert to using time.Duration in most places?
- Interactive CLI?
- Build an interactive web server, websockets and files embedded
- Use a templating engine for html
- Better handle the facebook/time override

Uses a [fork](https://github.com/cappe987/facebook-time) of
[facebook/time](https://github.com/facebook/time) for onestep support and
.Nano() function for timestamps.

Build InTime

```bash
# Host architecture
go build
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

