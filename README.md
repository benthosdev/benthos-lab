Benthos Lab
===========

This is an experimental site using a WASM build of Benthos for testing pipelines
in the browser.

This repo is subject to stagnation, modification beyond recognition, outright
deletion. I'm basically just messing about for fun, please don't get mad.

### Build

``` sh
# Build client
GOOS=js GOARCH=wasm go build -o ./client/wasm/benthos-lab.wasm ./client/wasm/benthos-lab.go

# Install server
go install ./server/benthos-lab
```

### Run

``` sh
cd ./client && benthos-lab
```

Then open your browser at `http://localhost:8080`.
