########################################################################################
# Build
########################################################################################

FROM golang:1.23.8-bullseye AS build

ENV GOBIN=/go/bin
ENV GOPATH=/go
ENV GOOS=linux

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /go/bin/diff-writer ./cmd/diff-writer
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /go/bin/diff-api ./cmd/diff-api

########################################################################################
# Deploy
########################################################################################

FROM debian:bullseye-20241111-slim@sha256:60a596681410bd31a48e5975806a24cd78328f3fd6b9ee5bc64dca6d46a51f29

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      jq \
      libffi-dev \
      libssl-dev \
      openssl \
      procps \
      python3 \
      python3-pip && \
    rm -rf /var/lib/apt/lists/* && \
    pip3 install --no-cache-dir requests==2.32.3 web3==7.5.0 retry==0.9.2

# Copy the compiled binaries over.
COPY --from=build /go/bin/diff-writer /usr/local/bin/diff-writer
COPY --from=build /go/bin/diff-api /usr/local/bin/diff-api

# default to fullnode
CMD ["/scripts/fullnode.sh"]
