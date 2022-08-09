# Copyright (c) 2022 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Stage 1: build the target binaries
FROM registry.ci.openshift.org/stolostron/builder:go1.18-linux AS builder

WORKDIR /workspace
COPY go.sum go.mod ./
COPY ./operator ./operator
COPY ./pkg ./pkg

RUN CGO_ENABLED=0 go build -a -installsuffix cgo -o bin/hub-of-hubs-operator operator/main.go

# Download console charts
RUN git clone https://github.com/stolostron/hub-of-hubs-console-chart.git && \
    git clone https://github.com/stolostron/hub-of-hubs-grc-chart.git && \
    git clone https://github.com/stolostron/application-chart.git

# Stage 2: Copy the binaries from the image builder to the base image
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/hub-of-hubs-operator \
    USER_UID=1001 \
    USER_NAME=operator

# install operator binary
COPY --from=builder /workspace/bin/hub-of-hubs-operator ${OPERATOR}

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
RUN mkdir .cache && chown ${USER_UID}:0 .cache && chmod 0777 .cache

USER ${USER_UID}
ENTRYPOINT ["/usr/local/bin/hub-of-hubs-operator"]
