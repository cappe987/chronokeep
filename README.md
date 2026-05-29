# InTime

**TODO:**
- Log levels
- Fix htons in port.go
- Add option to enable port recording later for TE mode
- Convert to using time.Duration in most places?

Uses a [fork](https://github.com/cappe987/facebook-time) of
[facebook/time](https://github.com/facebook/time) for onestep support and
.Nano() function for timestamps.


Cross-compile for ARM
```
GOARCH=arm go build -ldflags "-s -w"
```

-s strips binary

-w removes DWARF debugging info

For ARMv5
```
GOARCH=arm GOARM=5 go build -ldflags "-s -w"
```
