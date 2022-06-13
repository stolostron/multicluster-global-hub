# Copyright Contributors to the Open Cluster Management project

# Stage 1: Create image builder environment
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest AS go-env

# gcc for C
RUN microdnf install -y gcc make && rm -rf /var/lib/apt/lists/*
# tar and gzip for unpacking golang
RUN microdnf install tar gzip
#install golang binaries
ENV GOLANG_VERSION 1.17.5
ENV GOLANG_DOWNLOAD_URL https://golang.org/dl/go$GOLANG_VERSION.linux-amd64.tar.gz

RUN curl -fsSL "$GOLANG_DOWNLOAD_URL" -o golang.tar.gz \
    && tar -C /usr/local -xzf golang.tar.gz \
    && rm golang.tar.gz

RUN microdnf clean all

# add to PATH variable
ENV PATH /usr/local/go/bin:$PATH

# Stage 2: Use image builder to build the target binaries
FROM go-env AS builder

ARG COMPONENT
WORKDIR /workspace/${COMPONENT}
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY Makefile Makefile
COPY cmd/ cmd/
COPY pkg/ pkg/
RUN make build

# Stage 3: Copy the binaries from the image builder to the base image
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
