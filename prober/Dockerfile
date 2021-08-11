ARG DOCKER_PROXY_REGISTRY=""
FROM ${DOCKER_PROXY_REGISTRY}golang:1.16  as builder

LABEL maintainer="coreeng@us.ibm.com" \
      description="This docker image is used for the prober Go application"

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=undefined
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on \
    go build \
    -ldflags "-X main.Version=$VERSION" \
    -a \
    -o prober .

ARG DOCKER_PROXY_REGISTRY=""
FROM ${DOCKER_PROXY_REGISTRY}debian:buster-slim

RUN apt-get update && apt-get install -y curl net-tools jq && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /

COPY --from=builder /build/prober .

RUN addgroup --gid 901 prober && adduser --uid 901 --gid 901 prober && \
    chown prober:prober /prober && chmod u+x /prober

USER prober

CMD ["./prober"]
