#!/bin/bash

./scripts/veth.sh create

# -count=1 disables caching
ARGS="-count=1"

# Set -p=1 to not run packages concurrently. Since they use the same veth pair
# it can cause issues. If concurrency is needed, implement so each package sets
# up its own namespace.
if [ "$#" = "0" ]; then
    go test -v ./... -p=1 "$ARGS"
else
    go test -v "./$1" -p=1 "$ARGS"
fi
