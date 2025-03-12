# Copyright (c) 2022 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

-include /opt/build-harness/Makefile.prow

REGISTRY ?= quay.io/stolostron
IMAGE_TAG ?= latest
TMP_BIN ?= /tmp/cr-tests-bin
GO_TEST ?= go test -v
GIT_COMMIT := $(shell git rev-parse --short HEAD)

.PHONY: vendor			##download all third party libraries and puts them inside vendor directory
vendor:
	@go mod vendor

.PHONY: tidy			
tidy:
	@go mod tidy

.PHONY: clean-vendor			##removes third party libraries from vendor directory
clean-vendor:
	-@rm -rf vendor

build-operator-image: vendor
	cd operator && make
	docker build -t ${REGISTRY}/multicluster-global-hub-operator:${IMAGE_TAG} . -f operator/Dockerfile --build-arg GIT_COMMIT=$(GIT_COMMIT)

push-operator-image:
	docker push ${REGISTRY}/multicluster-global-hub-operator:${IMAGE_TAG}

deploy-operator: 
	cd operator && make deploy

undeploy-operator:
	cd operator && make undeploy

build-manager-image: vendor
	cd manager && make
	docker build -t ${REGISTRY}/multicluster-global-hub-manager:${IMAGE_TAG} . -f manager/Dockerfile --build-arg GIT_COMMIT=$(GIT_COMMIT)

push-manager-image:
	docker push ${REGISTRY}/multicluster-global-hub-manager:${IMAGE_TAG}

build-agent-image: vendor
	cd agent && make
	docker build -t ${REGISTRY}/multicluster-global-hub-agent:${IMAGE_TAG} . -f agent/Dockerfile --build-arg GIT_COMMIT=$(GIT_COMMIT)

push-agent-image:
	docker push ${REGISTRY}/multicluster-global-hub-agent:${IMAGE_TAG}

.PHONY: unit-tests
unit-tests: unit-tests-pkg unit-tests-operator unit-tests-manager unit-tests-agent

setup_envtest:
	GOBIN=${TMP_BIN} go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

unit-tests-operator: setup_envtest
	KUBEBUILDER_ASSETS="$(shell ${TMP_BIN}/setup-envtest use --use-env -p path)" ${GO_TEST} `go list ./operator/... | grep -v test`

unit-tests-manager: setup_envtest
	KUBEBUILDER_ASSETS="$(shell ${TMP_BIN}/setup-envtest use --use-env -p path)" ${GO_TEST} `go list ./manager/... | grep -v test`

unit-tests-agent: setup_envtest
	KUBEBUILDER_ASSETS="$(shell ${TMP_BIN}/setup-envtest use --use-env -p path)" ${GO_TEST} `go list ./agent/... | grep -v test`

unit-tests-pkg: setup_envtest
	KUBEBUILDER_ASSETS="$(shell ${TMP_BIN}/setup-envtest use --use-env -p path)" ${GO_TEST} `go list ./pkg/... | grep -v test`

.PHONY: fmt				##formats the code
fmt:
	@go fmt ./agent/... ./manager/... ./operator/... ./pkg/... ./test/...
	git diff --exit-code
	! grep -ir "multicluster-global-hub/agent/\|multicluster-global-hub/operator/\|multicluster-global-hub/manager/" ./pkg
	! grep -ir "multicluster-global-hub/agent/\|multicluster-global-hub/manager/" ./operator
	! grep -ir "multicluster-global-hub/manager/\|multicluster-global-hub/operator/" ./agent | grep -v "multicluster-global-hub/operator/api"
	! grep -ir "multicluster-global-hub/agent/\|multicluster-global-hub/operator/" ./manager | grep -v "multicluster-global-hub/operator/api"

.PHONY: strict-fmt				##formats the code
strict-fmt:
	@gci write -s standard -s default -s "prefix(github.com/stolostron/multicluster-global-hub)" ./agent/ ./manager/ ./operator/ ./pkg/ ./test/
	gofumpt -w ./agent/ ./manager/ ./operator/ ./pkg/ ./test/
	git diff --exit-code

# Include the e2e an integration makefile.
include ./test/Makefile
