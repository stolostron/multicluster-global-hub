# Copyright Contributors to the Open Cluster Management project

# Stage 1: build the target binaries
FROM registry.ci.openshift.org/stolostron/builder:go1.25-linux AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
COPY ./manager/ ./manager/
COPY ./operator/api ./operator/api
COPY ./pkg/ ./pkg/

RUN go build -o bin/manager ./manager/cmd/main.go

# Stage 2: Copy the binaries from the image builder to the base image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
ARG GIT_COMMIT
ENV USER_UID=1001
ENV USER_NAME=manager
ENV GIT_COMMIT=${GIT_COMMIT}

# install operator binary
COPY --from=builder /workspace/bin/manager /usr/local/bin/manager

COPY ./manager/scripts/user_setup /usr/local/scripts/user_setup
RUN /usr/local/scripts/user_setup

RUN microdnf update -y && microdnf clean all

USER ${USER_UID}
ENTRYPOINT ["/usr/local/bin/manager"]
