FROM golang:1.14 AS build

WORKDIR /go/src/github.com/benthosdev/benthos-lab/
COPY . /go/src/github.com/benthosdev/benthos-lab/

ENV GO111MODULE on
RUN CGO_ENABLED=0 GOOS=linux go build -o ./benthos-lab ./server/benthos-lab \
  && GOOS=js GOARCH=wasm go build -ldflags="-s -w -X main.Version=`cat ./benthos_version`" -o ./client/wasm/benthos-lab.wasm ./client/wasm/benthos-lab.go \
  && useradd -u 10001 benthos

FROM busybox AS package

LABEL maintainer="Ashley Jeffs <ash@jeffail.uk>"

COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /go/src/github.com/benthosdev/benthos-lab/benthos-lab .
COPY --from=build /go/src/github.com/benthosdev/benthos-lab/client /var/www

USER benthos

EXPOSE 8080

ENTRYPOINT ["/benthos-lab"]

CMD ["--www", "/var/www"]
