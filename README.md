<!-- SPDX-License-Identifier: MIT -->
<!-- SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com> -->

![ChronoKeep](app/static/logo/logo-grey-vectorized.svg)

Measure, test, and debug PTP. Send individual packets or run a whole
server/client setup that calculates time error and latencies. The application
must run on a device with two synchronised PTP-capable ports, with traffic
isolated to not cause a broadcast storm. ChronoKeep comes with both CLI and Web
depending on your use case. Web can display graphs directly, but both allows for
exporting the data and generating a PDF externally. Everything except PDF
generation is embedded in a single binary.

It can run without dedicated hardware support, but the results will not be
accurate and software timestamping must be explicitly selected. See <a
href="#testing">Testing</a> section.

Uses a [fork](https://github.com/cappe987/facebook-time) of
[facebook/time](https://github.com/facebook/time) for onestep support and
.Nano() function for timestamps.

## Getting Started

```bash
go build
sudo ./ckeep web
```


## Webgui


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
sudo ./ckeep web
```

And access it via localhost:8080.

Or use via CLI

```bash
sudo ./ckeep te -S -f configs/te.toml
```

TE mode relies a lot on config file to handle configuration of the two ports
separately.

## Generate PDF

A `measurement.dat` file is generated when Time Error capture is stopped.
Running the plotting script on it will create the file `output.pdf`.

```bash
python3 scripts/plot.py measurement.dat
```


## TODO
- Log levels
- Convert to using time.Duration in most places?
- Add documentation
- Add images to README
- Configurable .dat export
- Test more with UDP more
- Fix UDP multicast?
- Write tests. How?

Web:
- Allow web to load initial state from file.
- Help page that describes how things are calculated
- Indicate when capture starts
- Allow downloading data file for external graph generation
- Better error handling for invalid config
- Template dependency graph?
- Pkt mode:
  - Allow crafting specific sequences
  - Setting whether it should auto-reply to (P)Delays
  - Config file that supports similar behavior
- Wiretime mode?
  - A port of my wiretime project. Can theoretically be done in TE mode but it's
    nicer to have a dedicated mode.
- Ptpmonitor mode, like my old private PoC
  https://github.com/cappe987/ptpmonitor that uses expvar and the Linuxptp patch
  for `ptpmon` to transmit data from `ptp4l` to a TCP server. Display with
  chart.js?
  
## Credits

Logo uses the font `Gayathri`. Logo design by me.
