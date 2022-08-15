# Copyright (c) 2022 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

-include /opt/build-harness/Makefile.prow

REGISTRY ?= quay.io/stolostron
IMAGE_TAG ?= latest
TMP_BIN ?= /tmp/cr-tests-bin

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
	docker build -t ${REGISTRY}/multicluster-globalhub-operator:${IMAGE_TAG} . -f operator/Dockerfile

push-operator-image:
	docker push ${REGISTRY}/multicluster-globalhub-operator:${IMAGE_TAG}

deploy-operator: 
	cd operator && make deploy

build-manager-image: vendor
	cd manager && make
	docker build -t ${REGISTRY}/multicluster-globalhub-manager:${IMAGE_TAG} . -f manager/Dockerfile

push-manager-image:
	docker push ${REGISTRY}/multicluster-globalhub-manager:${IMAGE_TAG}

build-agent-image: vendor
	cd agent && make
	docker build -t ${REGISTRY}/multicluster-globalhub-agent:${IMAGE_TAG} . -f agent/Dockerfile

push-agent-image:
	docker push ${REGISTRY}/multicluster-globalhub-agent:${IMAGE_TAG}

.PHONY: unit-tests
unit-tests: unit-tests-operator unit-tests-manager unit-tests-agent

setup_envtest:
	GOBIN=${TMP_BIN} go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
	export KUBEBUILDER_ASSETS=`${TMP_BIN}/setup-envtest use --use-env -p path`

unit-tests-operator:
	go test `go list ./operator/... | grep -v test`

unit-tests-manager:
	go test `go list ./manager/... | grep -v test`

unit-tests-agent: setup_envtest
	go test `go list ./agent/... | grep -v test`

e2e-setup-dependencies:
	./test/setup/e2e_dependencies.sh

e2e-setup-start: e2e-setup-dependencies
	./test/setup/e2e_setup.sh

e2e-setup-clean:
	./test/setup/e2e_clean.sh

e2e-tests-all: tidy vendor
	./cicd-scripts/run-local-e2e-test.sh -v $(VERBOSE)

e2e-tests-connection e2e-tests-cluster e2e-tests-label e2e-tests-app e2e-tests-policy e2e-tests-local-policy: tidy vendor
	./cicd-scripts/run-local-e2e-test.sh -f $@ -v $(VERBOSE)

e2e-prow-tests: 
	./cicd-scripts/run-prow-e2e-test.sh

.PHONY: fmt				##formats the code
fmt:
	@gci write -s standard -s default -s "prefix(github.com/stolostron/multicluster-globalhub)" ./agent/ ./manager/ ./operator/ ./pkg/ ./test/pkg/
	@go fmt ./agent/... ./manager/... ./operator/... ./pkg/... ./test/pkg/...
	GOFUMPT_SPLIT_LONG_LINES=on gofumpt -w ./agent/ ./manager/ ./operator/ ./pkg/ ./test/pkg/
