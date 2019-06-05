#!/bin/sh
VER_FLAG="-X main.Version=`cat benthos_version`"
GOOS=js GOARCH=wasm go build -ldflags="-s -w $VER_FLAG" -o ./client/wasm/benthos-lab.wasm ./client/wasm/benthos-lab.go
go run ./server/benthos-lab --www ./client
