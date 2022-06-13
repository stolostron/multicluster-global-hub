# Stage 0: Install golang binaries on the base image
# Copyright Contributors to the Open Cluster Management project

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

#add to PATH variable
ENV PATH /usr/local/go/bin:$PATH

# Stage 1: Use image builder to build the target binaries
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

# allow user to fully control the /opt/kafka directory (for transport certificates)
RUN mkdir /opt/kafka
RUN chown ${USER_UID}:0 /opt/kafka
RUN chmod 0775 /opt/kafka

USER ${USER_UID}
ENTRYPOINT ["/usr/local/bin/manager"]
