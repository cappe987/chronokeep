# Intime ???


Uses a [fork](https://github.com/cappe987/facebook-time) of [facebook/time](https://github.com/facebook/time) for onestep support.


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
