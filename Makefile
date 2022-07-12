# Copyright (c) 2022 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

-include /opt/build-harness/Makefile.prow

REGISTRY ?= quay.io/stolostron
IMAGE_TAG ?= latest

.PHONY: vendor			##download all third party libraries and puts them inside vendor directory
vendor:
	@go mod vendor

.PHONY: clean-vendor			##removes third party libraries from vendor directory
clean-vendor:
	-@rm -rf vendor

build-operator-image: vendor
	cd operator && make
	docker build -t ${REGISTRY}/hub-of-hubs-operator:${IMAGE_TAG} . -f operator/Dockerfile

push-operator-image:
	docker push ${REGISTRY}/hub-of-hubs-operator:${IMAGE_TAG}

build-manager-image: vendor
	cd manager && make
	docker build -t ${REGISTRY}/hub-of-hubs-manager:${IMAGE_TAG} . -f manager/Dockerfile

push-manager-image:
	docker push ${REGISTRY}/hub-of-hubs-manager:${IMAGE_TAG}

build-agent-image: vendor
	cd agent && make
	docker build -t ${REGISTRY}/hub-of-hubs-agent:${IMAGE_TAG} . -f agent/Dockerfile

push-agent-image:
	docker push ${REGISTRY}/hub-of-hubs-agent:${IMAGE_TAG}

.PHONY: unit-tests
unit-tests: unit-tests-manager unit-tests-agent

unit-tests-operator:
	go test `go list ./operator/... | grep -v test`

unit-tests-manager:
	go test `go list ./manager/... | grep -v test`

unit-tests-agent:
	go test `go list ./agent/... | grep -v test`

e2e-setup-start:
	./test/setup/e2e_setup.sh

e2e-setup-clean:
	./test/setup/e2e_clean.sh

e2e-tests-all:
	./cicd-scripts/run-local-e2e-test.sh -v $(verbose)

e2e-tests-connection e2e-tests-cluster e2e-tests-label e2e-tests-app e2e-tests-policy e2e-tests-local-policy:
	./cicd-scripts/run-local-e2e-test.sh -f $@ -v $(verbose)

e2e-prow-tests:
	./cicd-scripts/run-prow-e2e-test.sh