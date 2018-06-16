FROM golang:1.9 AS builder

# TODO vendoring
RUN go get github.com/jroimartin/gocui

COPY . "/go/src/github.com/ulmenhaus/env/img/jql"

WORKDIR "/go/src/github.com/ulmenhaus/env/img/jql"

RUN go build -o /jql ./cmd/jql/


FROM alpine

COPY --from=builder /jql /jql

# HACK sleeping 1 seems to give the tty device proper
# time to set up
ENTRYPOINT ["sh", "-c", "sleep 1 && /jql"]
