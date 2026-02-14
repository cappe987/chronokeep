



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
