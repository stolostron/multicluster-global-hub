# Copyright Contributors to the Open Cluster Management project

# Stage 1: build the target binaries
FROM registry.ci.openshift.org/stolostron/builder:go1.19-linux AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
COPY ./agent/ ./agent/
COPY ./pkg/ ./pkg/

RUN go build -o bin/agent ./agent/cmd/agent/main.go
RUN go build -o bin/applier ./agent/cmd/applier/main.go

# Stage 2: Copy the binaries from the image builder to the base image
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
ENV USER_UID=1001
ENV USER_NAME=agent

# install operator binary
COPY --from=builder /workspace/bin/agent /usr/local/bin/agent
COPY --from=builder /workspace/bin/applier /usr/local/bin/applier

COPY ./agent/scripts/user_setup /usr/local/scripts/user_setup
RUN  /usr/local/scripts/user_setup

RUN microdnf update && \
    microdnf clean all

USER ${USER_UID}
ENTRYPOINT ["/usr/local/bin/agent"]
