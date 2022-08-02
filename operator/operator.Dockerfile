# Build the hub-of-hubs-operator binary
FROM golang:1.18 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY operator/main.go operator/main.go
COPY operator/apis/ operator/apis/
COPY operator/pkg/ operator/pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o hub-of-hubs-operator operator/main.go

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
WORKDIR /
COPY --from=builder /workspace/hub-of-hubs-operator /usr/local/bin/hub-of-hubs-operator
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/hub-of-hubs-operator"]
