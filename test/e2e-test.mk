
e2e-dep: 
	./test/scripts/e2e_dep.sh

e2e-setup: tidy vendor e2e-dep
	./test/scripts/e2e_setup.sh

e2e-cleanup:
	./test/scripts/e2e_clean.sh

e2e-test-all: tidy vendor
	./test/scripts/run_e2e.sh -f "e2e-test-validation,e2e-test-localpolicy,e2e-test-placement,e2e-test-app,e2e-test-policy,e2e-tests-backup" -v $(VERBOSE)

e2e-test-cluster e2e-test-placement e2e-test-app e2e-test-policy e2e-test-localpolicy e2e-test-grafana: tidy vendor
	./test/scripts/e2e_run.sh -f $@ -v $(VERBOSE)

e2e-test-prune: tidy vendor
	./test/scripts/e2e_run.sh -f "e2e-test-prune" -v $(VERBOSE)

e2e-prow-tests: 
	./test/scripts/e2e_prow.sh