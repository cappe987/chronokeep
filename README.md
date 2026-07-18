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
accurate and software timestamping must be explicitly selected.


## Getting Started

This builds the application, creates veth ports `veth1` and `veth2`, and starts
up the web server on `localhost:8080` with a predefined config for running with
software timestamping on the veth ports.

```bash
go build
sudo ./scripts/veth.sh create
sudo ./ckeep web -f configs/te.toml
```

Using it without sudo requires `CAP_NET_RAW` for L2 operation, and
`CAP_NET_BIND_SERVICE` for UDP operation (PTP uses ports 319 and 320).


## Webgui


## Build options

- No web: `go build --tags noweb`
- Stripped: `go build -ldflags "-s -w"`
- ARM64: `GOARCH=arm64 go build`
- ARMv7: `GOARCH=arm go build`
- ARMv5: `GOARCH=arm GOARM=5 go build`

No web server and stripping debug info results in a much smaller binary, if
desired. 14 MB vs 2.8 MB at the time of writing.


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

The project uses a [fork](https://github.com/cappe987/facebook-time) of
[facebook/time](https://github.com/facebook/time) for onestep support and
.Nano() function for timestamps.
