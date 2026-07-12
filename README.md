# InTime

**TODO:**
- Add license to project. MIT?
- Rename project?
- Log levels
- Convert to using time.Duration in most places?
- Add documentation
- Add images to README
- Add proper description in this README

Web:
- Help page that describes how things are calculated
- Indicate when capture starts
- Allow downloading data file for external graph generation
- Better error handling for invalid config
- More modes. Block user from swapping while one is active
- Template dependency graph?

Uses a [fork](https://github.com/cappe987/facebook-time) of
[facebook/time](https://github.com/facebook/time) for onestep support and
.Nano() function for timestamps.

## Building

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

## Testing

Getting proper values requires dedicated hardware. But using Veth ports is a
good way to test the functionality without focusing too much on the actual
results. To run it you must also select SwTstamp (Web) or `-S` (CLI) to enable
software timestamping, otherwise it will fail to timestamp.

```bash
sudo ip link add dev veth1 type veth peer name veth2
sudo ip link set dev veth1 up
sudo ip link set dev veth2 up
sudo ./intime web
```

And access it via localhost:8080.

Or use via CLI

```bash
sudo ./intime te -S -f configs/te.toml
```

TE mode relies a lot on config file to handle configuration of the two ports
separately.

