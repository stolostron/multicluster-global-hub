FROM registry.ci.openshift.org/stolostron/builder:go1.18-linux AS builder

WORKDIR /workspace
# copy go tests into build image
COPY go.sum go.mod ./
COPY ./test ./test
COPY ./pkg ./pkg
COPY ./operator ./operator
COPY ./manager ./manager
COPY ./agent ./agent

# compile go tests in build image
RUN go install github.com/onsi/ginkgo/ginkgo@v1.14.2 && go mod vendor && ginkgo build test/pkg/e2e

# create new docker image to hold built artifacts
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

# run as root
USER root

# expose env vars for runtime
ENV LOG "/dev/stdout"
ENV LEAF_HUB_LOG "/dev/stdout"
ENV VERBOSE "6"
ENV KUBECONFIG "/opt/.kube/config"
ENV IMPORT_KUBECONFIG "/opt/.kube/import-kubeconfig"
ENV OPTIONS "/resources/options.yaml"
ENV REPORT_FILE "/results/results.xml"
ENV GINKGO_DEFAULT_FLAGS "-slowSpecThreshold=120 -timeout 7200s"
ENV GINKGO_NODES "1"
ENV GINKGO_FLAGS=""
ENV GINKGO_FOCUS=""
ENV GINKGO_SKIP="Integration"
ENV SKIP_INTEGRATION_CASES="true"
ENV IS_CANARY_ENV="true"

# install ginkgo into built image
COPY --from=builder /go/bin/ /usr/local/bin

# oc exists in the base image. copy oc into built image
COPY --from=builder /usr/local/bin/oc /usr/local/bin/oc
RUN oc version

WORKDIR /workspace/opt/test/
# copy compiled tests into built image
COPY --from=builder /workspace/test/pkg/e2e/e2e.test ./hoh-e2e-test.test
COPY --from=builder /workspace/test/format-results.sh .

VOLUME /results


# execute compiled ginkgo tests
# CMD ["/bin/bash", "-c", "ginkgo --v --focus=${GINKGO_FOCUS} --skip=${GINKGO_SKIP} -nodes=${GINKGO_NODES} --reportFile=${REPORT_FILE} -x -debug -trace hoh-e2e-test.test -- -v=3 && ./format-results.sh ${REPORT_FILE}"]
CMD ["/bin/bash", "-c", "ginkfo --reportFile=${REPORT_FILE} -x -debug -trace hoh-e2e-test.test -- -options=$OPTIONS_FILE -v=3 && ./format-results.sh ${REPORT_FILE}"]
# docker run --volume ~/.kube/config:/opt/.kube/config --volume /home/cloud-user/multicluster-global-hub/test/resources/options-local.yaml:/resources/options.yaml -it canary /bin/bash