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
