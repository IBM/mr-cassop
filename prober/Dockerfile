ARG DOCKER_PROXY_REGISTRY=""
FROM ${DOCKER_PROXY_REGISTRY}golang:alpine as builder

LABEL maintainer="coreeng@us.ibm.com" \
      description="This docker image is used for the prober Go application" \
      version="golang:alpine"

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
FROM ${DOCKER_PROXY_REGISTRY}alpine:3.13.1

RUN apk add --update --no-cache ca-certificates curl net-tools jq && \
    rm -rf /var/cache/apk/*

WORKDIR /

COPY --from=builder /build/prober .

RUN addgroup -S prober && adduser -S prober -G prober && \
    chown prober:prober /prober && chmod u+x /prober

USER prober

CMD ["./prober"]
