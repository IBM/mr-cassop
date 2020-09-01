FROM golang:1.15 as builder

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

ARG VERSION=undefined

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on \
    go build \
    -ldflags "-X main.Version=$VERSION" \
    -a
    -o bin/cassandra-operator main.go

FROM debian:buster-slim

WORKDIR /

RUN addgroup --gid 901 cassandra-operator && adduser --uid 901 --gid 901 cassandra-operator

COPY --from=builder /workspace/bin/cassandra-operator .
USER cassandra-operator

ENTRYPOINT ["/cassandra-operator"]
