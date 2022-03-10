ARG DOCKER_PROXY_REGISTRY=""
FROM ${DOCKER_PROXY_REGISTRY}golang:1.17  as builder

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
FROM ${DOCKER_PROXY_REGISTRY}debian:bookworm-slim

WORKDIR /

RUN apt-get update && \
    apt-get install -y ca-certificates && \
    update-ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN addgroup --gid 901 prober && adduser --uid 901 --gid 901 prober

USER prober

COPY --from=builder /build/prober .

CMD ["./prober"]
