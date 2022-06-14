
 verbose ?= 5

.PHONY: fmt				
fmt:
	@gci overwrite ./pkg/
	@go fmt ./pkg/...
	@gofumpt -w ./pkg/

.PHONY: check
check: fmt
	@../cicd-scripts/check-variables.sh

connection label policy app : fmt check
	@../cicd-scripts/run-tests.sh -f $@ -v $(verbose)

.PHONY: all
all: fmt check
	@../cicd-scripts/run-tests.sh -f "connection && label && policy && app" -v $(verbose)

.PHONY: vendor			
vendor:
	@go mod vendor

.PHONY: clean-vendor		
clean-vendor:
	-@rm -rf vendor
