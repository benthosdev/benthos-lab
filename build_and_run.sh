#!/bin/sh
GOOS=js GOARCH=wasm go build -o ./client/wasm/benthos-lab.wasm ./client/wasm/benthos-lab.go
go run ./server/benthos-lab --www ./client
