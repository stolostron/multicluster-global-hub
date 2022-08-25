# Copyright Contributors to the Open Cluster Management project

# Stage 1: build the target binaries
FROM registry.ci.openshift.org/stolostron/builder:go1.18-linux AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
COPY ./manager/ ./manager/
COPY ./pkg/ ./pkg/

RUN go build -o bin/manager ./manager/cmd/manager/main.go

# Stage 2: Copy the binaries from the image builder to the base image
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
ENV USER_UID=1001
ENV USER_NAME=manager

# install operator binary
COPY --from=builder /workspace/bin/manager /usr/local/bin/manager
COPY ./manager/scripts/prestop.sh /usr/local/bin/prestop.sh
RUN curl -sLO https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64 && chmod 755 jq-linux64 
RUN curl -sL https://raw.githubusercontent.com/stedolan/jq/master/sig/v1.6/sha256sum.txt -o jq.sha256sum && \
    grep -e "jq-linux64$" jq.sha256sum | sha256sum -c && mv jq-linux64 /usr/local/bin/jq && rm jq.sha256sum

COPY ./manager/scripts/user_setup /usr/local/scripts/user_setup
RUN  /usr/local/scripts/user_setup

RUN microdnf update && \
    microdnf clean all

# allow user to fully control the /opt/kafka directory (for transport certificates)
RUN mkdir /opt/kafka
RUN chown ${USER_UID}:0 /opt/kafka
RUN chmod 0775 /opt/kafka

USER ${USER_UID}
ENTRYPOINT ["/usr/local/bin/manager"]
