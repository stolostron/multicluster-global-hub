# Copyright Contributors to the Open Cluster Management project

# -------------------------------------------------------------
# This makefile defines the following targets
#
#   - all (default) - formats the code, runs liners, downloads vendor libs, and builds executable
#   - fmt - formats the code
#   - lint - runs code analysis tools
#   - vendor - download all third party libraries and puts them inside vendor directory
#   - clean-vendor - removes third party libraries from vendor directory
#   - build - builds the binary
#   - build-images - builds docker image locally for running the components using docker
#   - push-images - pushes the local docker image to docker registry
#   - clean - cleans the build directories
#   - clean-all - superset of 'clean' that also removes vendor dir

COMPONENT := $(shell basename $(shell pwd))
REGISTRY ?= quay.io/stolostron
IMAGE_TAG ?= latest
IMAGE := ${REGISTRY}/hub-of-hubs-${COMPONENT}:${IMAGE_TAG}

.PHONY: all				##formats the code, runs liners, downloads vendor libs, and builds executable
all: fmt lint build

.PHONY: fmt				##formats the code
fmt:
	@gci -w ./cmd/ ./pkg/
	@go fmt ./cmd/... ./pkg/...
	@gofumpt -w ./cmd/ ./pkg/

CONTROLLER_GEN = controller-gen
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

.PHONY: controller-gen		##downloads controller-gen locally if necessary.
controller-gen:
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.1

.PHONY: generate			##generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests			##generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=deploy/crd

.PHONY: build			##builds the binary
build:
	@go build -o bin/${COMPONENT} cmd/manager/main.go

.PHONY: build-image			##builds docker image locally for running the components using docker
build-image: all
	docker build -t ${IMAGE} --build-arg COMPONENT=${COMPONENT} -f Dockerfile .

.PHONY: push-image			##pushes the local docker image to docker registry
push-image: build-image
	@docker push ${IMAGE}

.PHONY: clean			##cleans the build directories
clean:
	@rm -rf bin

.PHONY: clean-all			##superset of 'clean' that also removes vendor dir
clean-all: clean-vendor clean

.PHONY: lint				##runs code analysis tools
lint:
	go vet ./cmd/... ./pkg/...
	golint ./cmd/... ./pkg/...
	golangci-lint run ./cmd/... ./pkg/...

.PHONY: help				##show this help message
help:
	@echo "usage: make [target]\n"; echo "options:"; \fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//' | sed 's/.PHONY:*//' | sed -e 's/^/  /'; echo "";