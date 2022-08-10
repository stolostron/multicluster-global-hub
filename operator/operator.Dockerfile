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

# Download console charts
RUN git clone https://github.com/stolostron/hub-of-hubs-console-chart.git && \
    git clone https://github.com/stolostron/hub-of-hubs-grc-chart.git && \
    git clone https://github.com/stolostron/application-chart.git

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
WORKDIR /
COPY --from=builder /workspace/hub-of-hubs-operator /usr/local/bin/hub-of-hubs-operator

RUN microdnf install jq tar gzip && \
    microdnf update && \
    microdnf clean all

# install kubectl
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl && \
    rm kubectl

# install helm
RUN curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
    chmod 700 get_helm.sh && \
    VERIFY_CHECKSUM=false ./get_helm.sh && \
    rm get_helm.sh

# install yq
RUN curl -LO https://github.com/mikefarah/yq/releases/download/v4.25.3/yq_linux_amd64 && \
    install -o root -g root -m 0755 yq_linux_amd64 /usr/local/bin/yq && \
    rm yq_linux_amd64

# copy the console charts
RUN mkdir -p charts
COPY --from=builder /workspace/hub-of-hubs-console-chart/stable/console-chart charts/console-chart 
COPY --from=builder /workspace/hub-of-hubs-grc-chart/stable/grc charts/grc
COPY --from=builder /workspace/application-chart/stable/application-chart charts/application-chart

# allow running user to create cache
RUN mkdir .cache && chown 65532:65532 .cache && chmod 0777 .cache

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/hub-of-hubs-operator"]
