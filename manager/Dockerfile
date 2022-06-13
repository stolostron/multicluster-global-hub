# Copyright Contributors to the Open Cluster Management project

# Stage 1: build the target binaries
FROM registry.ci.openshift.org/stolostron/builder:go1.17-linux AS builder

ARG COMPONENT
WORKDIR /workspace/${COMPONENT}

COPY go.mod go.sum ./
COPY ./manager/ ./manager/
COPY ./pkg/ ./pkg/

RUN CGO_ENABLED=0 go build -o bin/manager ./manager/cmd/manager/main.go

# Stage 2: Copy the binaries from the image builder to the base image
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
ARG COMPONENT
ENV USER_UID=1001
ENV USER_NAME=${COMPONENT}

# install operator binary
COPY --from=builder /workspace/${COMPONENT}/bin/${COMPONENT} /usr/local/bin/manager

COPY scripts/user_setup /usr/local/scripts/user_setup
RUN  /usr/local/scripts/user_setup

RUN microdnf update && \
    microdnf clean all

USER ${USER_UID}
ENTRYPOINT ["/usr/local/bin/manager"]
