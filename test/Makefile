
 verbose ?= 5

.PHONY: fmt				
fmt:
	@gci overwrite ./pkg/
	@go fmt ./pkg/...
	@gofumpt -w ./pkg/


connection label policy app : fmt
	@../cicd-scripts/run-e2e-tests.sh -f $@ -v $(verbose)

.PHONY: all
all: fmt
	@../cicd-scripts/run-e2e-tests.sh -f "connection && label && policy && app" -v $(verbose)

.PHONY: vendor			
vendor:
	@go mod vendor

.PHONY: clean-vendor		
clean-vendor:
	-@rm -rf vendor
