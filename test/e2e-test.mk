
e2e-dep: 
	./test/script/e2e_dep.sh

e2e-setup: tidy vendor e2e-dep
	./test/script/e2e_setup.sh

e2e-cleanup:
	./test/script/e2e_cleanup.sh

e2e-test-all: tidy vendor
	./test/script/e2e_run.sh -f "e2e-test-localpolicy,e2e-test-placement,e2e-test-app,e2e-test-policy,e2e-tests-backup" -v $(VERBOSE)

e2e-test-cluster e2e-test-placement e2e-test-app e2e-test-policy e2e-test-localpolicy e2e-test-grafana: tidy vendor
	./test/script/e2e_run.sh -f $@ -v $(VERBOSE)

e2e-test-prune: tidy vendor
	./test/script/e2e_run.sh -f "e2e-test-prune" -v $(VERBOSE)

e2e-prow-tests: 
	./test/script/e2e_prow.sh