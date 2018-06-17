FROM golang:1.9 AS builder

# TODO vendoring
RUN go get github.com/jroimartin/gocui

COPY . "/go/src/github.com/ulmenhaus/env/img/netlog"

WORKDIR "/go/src/github.com/ulmenhaus/env/img/netlog"

RUN go build -o /netlog ./cmd/netlog/


FROM alpine

RUN apk add --update tcpdump

COPY --from=builder /netlog /netlog

# HACK sleeping 1 seems to give the tty device proper
# time to set up
ENTRYPOINT ["sh", "-c", "sleep 1 && /netlog"]
