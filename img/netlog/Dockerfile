FROM golang:1.9

# TODO vendoing and multi-stage build
RUN go get github.com/jroimartin/gocui

COPY . "/go/src/github.com/ulmenhaus/env/img/netlog"

WORKDIR "/go/src/github.com/ulmenhaus/env/img/netlog"

RUN go build -o /netlog ./cmd/netlog/

ENTRYPOINT ["/netlog"]
