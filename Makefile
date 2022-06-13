# Copyright (c) 2022 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

REGISTRY ?= quay.io/stolostron
IMAGE_TAG ?= latest

.PHONY: vendor			##download all third party libraries and puts them inside vendor directory
vendor:
	@go mod vendor

.PHONY: clean-vendor			##removes third party libraries from vendor directory
clean-vendor:
	-@rm -rf vendor

build-operator-image: vendor
	cd operator && make build-image

push-operator-image:
	cd operator && make push-image

build-manager-image: vendor
	cd manager && make
	docker build -t ${REGISTRY}/hub-of-hubs-manager:${IMAGE_TAG} . -f manager/Dockerfile

push-manager-image:
	docker push ${REGISTRY}/hub-of-hubs-manager:${IMAGE_TAG}

build-agent-image: vendor
	cd agent && make build-image

push-agent-image:
	cd agent && make push-image

.PHONY: unit-tests
unit-tests: unit-tests-operator unit-tests-manager unit-tests-agent

unit-tests-operator:
	go test `go list ./operator/... | grep -v test`

unit-tests-manager:
	go test `go list ./manager/... | grep -v test`

unit-tests-agent:
	go test `go list ./agent/... | grep -v test`
