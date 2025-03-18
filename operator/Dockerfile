# Copyright (c) 2022 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Stage 1: build the target binaries
FROM registry.ci.openshift.org/stolostron/builder:go1.23-linux AS builder

WORKDIR /workspace
COPY go.sum go.mod ./
COPY ./operator ./operator
COPY ./pkg ./pkg

RUN CGO_ENABLED=0 go build -a -installsuffix cgo -o bin/multicluster-global-hub-operator operator/main.go

# Stage 2: Copy the binaries from the image builder to the base image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/multicluster-global-hub-operator \
    USER_UID=1001 \
    USER_NAME=operator

# install operator binary
COPY --from=builder /workspace/bin/multicluster-global-hub-operator ${OPERATOR}

RUN microdnf update -y && microdnf clean all

USER ${USER_UID}
ENTRYPOINT ["/usr/local/bin/multicluster-global-hub-operator"]
