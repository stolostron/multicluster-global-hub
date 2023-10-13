# Copyright (c) 2022 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

-include /opt/build-harness/Makefile.prow

REGISTRY ?= quay.io/stolostron
IMAGE_TAG ?= latest
TMP_BIN ?= /tmp/cr-tests-bin
GO_TEST ?= go test -v

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
	docker build -t ${REGISTRY}/multicluster-global-hub-operator:${IMAGE_TAG} . -f operator/Dockerfile

push-operator-image:
	docker push ${REGISTRY}/multicluster-global-hub-operator:${IMAGE_TAG}

deploy-operator: 
	cd operator && make deploy

undeploy-operator:
	cd operator && make undeploy

build-manager-image: vendor
	cd manager && make
	docker build -t ${REGISTRY}/multicluster-global-hub-manager:${IMAGE_TAG} . -f manager/Dockerfile

push-manager-image:
	docker push ${REGISTRY}/multicluster-global-hub-manager:${IMAGE_TAG}

build-agent-image: vendor
	cd agent && make
	docker build -t ${REGISTRY}/multicluster-global-hub-agent:${IMAGE_TAG} . -f agent/Dockerfile

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

e2e-setup-dependencies: 
	./test/setup/e2e_dependencies.sh

e2e-setup-start: tidy vendor e2e-setup-dependencies
	./test/setup/e2e_setup.sh

e2e-setup-clean:
	./test/setup/e2e_clean.sh

e2e-tests-all: tidy vendor
	./cicd-scripts/run-local-e2e-test.sh -f "e2e-tests-validation,e2e-tests-local-policy,e2e-tests-grafana,(e2e-tests-placement && !e2e-tests-global-resource)" -v $(VERBOSE)

e2e-tests-validation e2e-tests-label e2e-tests-placement e2e-tests-app e2e-tests-policy e2e-tests-local-policy: tidy vendor
	./cicd-scripts/run-local-e2e-test.sh -f $@ -v $(VERBOSE)

e2e-tests-prune: tidy vendor
	./cicd-scripts/run-local-e2e-test.sh -f "(e2e-tests-prune && !e2e-tests-global-resource)" -v $(VERBOSE)

e2e-tests-prune-all: tidy vendor
	./cicd-scripts/run-local-e2e-test.sh -f e2e-tests-prune -v $(VERBOSE)

e2e-prow-tests: 
	./cicd-scripts/run-prow-e2e-test.sh

.PHONY: fmt				##formats the code
fmt:
	@gci write -s standard -s default -s "prefix(github.com/stolostron/multicluster-global-hub)" ./agent/ ./manager/ ./operator/ ./pkg/ ./test/pkg/
	@go fmt ./agent/... ./manager/... ./operator/... ./pkg/... ./test/pkg/...
	gofumpt -w ./agent/ ./manager/ ./operator/ ./pkg/ ./test/pkg/
	git diff --exit-code

install-kafka: # install kafka on the ocp
	./operator/config/samples/transport/deploy_kafka.sh

uninstall-kafka: 
	./operator/config/samples/transport/undeploy_kafka.sh

install-postgres: # install postgres on the ocp
	./operator/config/samples/storage/deploy_postgres.sh

uninstall-postgres: 
	./operator/config/samples/storage/undeploy_postgres.sh