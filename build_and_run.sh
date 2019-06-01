#!/bin/sh
GOOS=js GOARCH=wasm go build -ldflags='-s -w' -o ./client/wasm/benthos-lab.wasm ./client/wasm/benthos-lab.go
go run ./server/benthos-lab --www ./client
