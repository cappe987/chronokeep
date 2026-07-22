
## Examples

### Packet Mode

Send a L2 Delay_Req packet on `veth1` using software timestamping (`-S`).

```bash
ckeep pkt -S -i veth1 delay_req
```

---

Listen for L2 packets on port `veth2` with software timestamping (`-S`).
```bash
ckeep pkt -S -r -i veth2
```

---

Send 5 packets at 10 ms interval from `veth1` with IP `10.11.0.1` to
destination IP `10.11.0.2` over UDP.

```bash
ckeep pkt -4 --sip 10.11.0.1 --dip 10.11.0.2 -i veth1 -c 5 -I 10
```

---

### Delay Mode

Start a delay server in L2 P2P mode. It replies to any PDelayReq it receives on the
configured domain.

```bash
ckeep delay --server -i veth1 -P
```

---

Start a delay client in L2 P2P mode. It sends PDelayReq at the configured interval.

```bash
ckeep delay --client -i veth2 -P
```

---

### GM Mode

Start a PTP GM in L2 P2P mode on domain 5 with onestep timestamping.

```bash
ckeep gm -i veth2 -P -d 5 -o
```

---

### Time Error Mode

Start Time Error mode using the config file. The config file specifies L2,
Software timestamping, transmit interval of 100 ms, ports `veth1` (GM) and
`veth2` (client), and exports the resulting data to `measurements.dat`. TE mode
relies on config file or web GUI, ports cannot be specified as CLI argument.

```bash
ckeep te -f configs/te.toml
```

---
