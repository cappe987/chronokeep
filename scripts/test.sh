#!/bin/bash

./scripts/veth.sh create

# -count=1 disables caching
go test -v ./...
