
## Time Error Mode

Measure the performance of a PTP switch. ChronoKeep is intended as a
development tool. The accuracy of the measurements depend on the accuracy
of the hardware it runs on. If the test platform is not known-good there is
no guarantee the measurements are correct. Use real validation tools to do
the final calibrations.

Select two ports. They must be synchronized (e.g. via
`ts2phc`) or be the same PHC. They should be on separate
forwarding planes (i.e. not the same VLAN/bridge). Port 1 becomes the PTP
GM. SwTstamp is for software timestamping.

```
+---------+
|  TC/BC  |
|eth1 eth2|
| +-----+ |
+-|-----|-+
  |     |
  |     |
+-|-----|-+
|eth1 eth2|
|  ckeep  |
+---------+
```

`Start-and-capture` is primarily useful for E2E TC. In this mode
the measurements can start immediately. A P2P TC may need a second or two to
calculate the first peer delay before, so use `Start` and shortly
after `Capture`. A BC may need anything from a couple up to like
20 seconds before it has synchronized. Check when your BC is ready before
starting capture. Stop capture after running for some time. Graphs are
generated at the bottom of the page, also containing mean/max/min values.

### T1 Time Error

Total difference between Sync origin timestamp and Sync RX timestamp on
client port. CorrectionField is included in calculation. A negative value is
expected because `t1` is taken before `t2`. E.g. a
T1TE of -500 means that 500 ns of inaccuracy was incurred over the DUT. If
this is an E2E TC, a positive T4TE of the same value is expected for the
final TwowayTE have a precise result. A positive T1TE indicates an
overcompensation by the DUT.

$$
t1\\_te = (t_1 - t_2) + c_1 + c_2
$$

For Transparent Clock, this is the total error incurred through the DUT.

For Boundary Clock, this is the difference between DUT and server/client
time (cable delay included).

### T4 Time Error

Only for E2E. Similar to T1 Time Error, but for Delay request.
`t3` is the timestamp of DelayReq being transmitted by the
client. `t4` is the timestamp of server or DUT (depending on TC
or BC) receiving the DelayReq. T4TE is expected to be positive. A negative
value indicates overcompensation by the DUT.

$$
t4\\_te = t_4 - t_3 - c_4 - c_3
$$

### Twoway Time Error

Only for E2E. When a DelayResp is received, the most recent T1TE and the
newly received T4TE is taken and used to calculate the TwowayTE at this
moment in time. This is the true error a PTP client would experience. If
this is not near zero then there is inaccuracy in the network. T1TE and T4TE
should be the same value, except T1TE being negative. If T1 and T4 differ, there
is likely an asymmetry in the ingress/egress latency of the ports' timestamping
blocks.

$$
twoway\\_te = \frac{t1\\_te + t4\\_te}{2}
$$

Note that two ports of the same media and speed can cancel out each others'
ingress/egress latency. Test different media and speeds for the two ports to
be more certain of the end result.

### PDelay Time Error

Only for P2P. Total time the peer delay was corrected for by the DUT. Uses
PDelayReq transmitted by server and PDelayResp/PDelayRespFup transmitted by
DUT.

$$
pdelay = \frac{(t_4 - t_1) - (t_3 - t_2) - c_1 - c_2}{2}
$$

### Forwarding Accuracy

Only for P2P. Similar to TwowayTE, this is the end result a client sees in
P2P mode. The peer delay is added to the already negative value of T1TE to
ideally bring that value to zero.

$$
fwd\\_accuracy = t1\\_te + pdelay
$$


### T1 Latency

The total time a Sync is on the wire. Subtract OriginTS from the client RX
timestamp. For Boundary Clock, this is the same as T1TE (except no
correctionField is included here).

$$
t1\\_lat = t_2 - t_1
$$

### T4 Latency

Only for E2E. Same as T1 Latency but for DelayReq.

$$
t4\\_lat = t_4 - t_3
$$

### PDelay Turnaround Latency

Only for P2P. Total time a peer delay transaction is on the wire. From when
the PDelayReq transmits by the client, to when the PDelayResp is received by
the client.

$$
pdelay\\_turnaround = t_4 - t_1
$$

---

## Packet Mode

Coming soon...
