# Examples

## Packet Mode

Listen for L2 packets on port `veth2` with software timestamping (`-S`).
```bash
./ckeep pkt -S -r -i veth2
```

Send 5 packets at 10 ms interval from `veth1` with IP `10.11.0.1` to
destination IP `10.11.0.2` over UDP.

```bash
./ckeep pkt -4 --sip 10.11.0.1 --dip 10.11.0.2 -i veth1 -c 5 -I 10
```
