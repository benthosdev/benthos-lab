FROM golang:1.12 AS build

COPY . /benthos-lab/
WORKDIR /benthos-lab

ENV GO111MODULE=on

RUN go get github.com/Jeffail/benthos/v3@master \
  && CGO_ENABLED=0 GOOS=linux go build -o ./benthos-lab ./server/benthos-lab \
  && GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o ./client/wasm/benthos-lab.wasm ./client/wasm/benthos-lab.go \
  && useradd -u 10001 benthos

FROM busybox AS package

LABEL maintainer="Ashley Jeffs <ash@jeffail.uk>"

COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /benthos-lab/benthos-lab .
COPY --from=build /benthos-lab/client /var/www

USER benthos

EXPOSE 8080

ENTRYPOINT ["/benthos-lab"]

CMD ["--www", "/var/www"]
