![benthos-lab](logo.svg "benthos lab")

[![Build Status](https://cloud.drone.io/api/badges/benthosdev/benthos-lab/status.svg)](https://cloud.drone.io/benthosdev/benthos-lab)

Benthos Lab is a web application for building, formatting, testing and sharing
[Benthos](https://www.benthos.dev/) pipeline configurations.

It contains a full version of the Benthos streaming engine compiled to Web
Assembly. This allows it to run natively within the browser sandbox rather than
on a hosted instance, this allows us to be relaxed in regards to allowing
certain processors and connectors to execute.

### Install

Pull a docker image with:

``` sh
docker pull jeffail/benthos-lab
```

### Build

``` sh
# Build client
GOOS=js GOARCH=wasm go build -ldflags='-s -w' -o ./client/wasm/benthos-lab.wasm ./client/wasm/benthos-lab.go

# Install server
go install ./server/benthos-lab
```

Docker:

``` sh
go mod vendor
docker build . -t jeffail/benthos-lab:latest
```

### Run

``` sh
cd ./client && benthos-lab
```

Docker:

``` sh
docker run --rm -p 8080:8080 jeffail/benthos-lab
```

Then open your browser at `http://localhost:8080`.
