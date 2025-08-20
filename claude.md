# Failed integration tests were found in the recent PRs
1. [FAIL] MigrationFromSyncer Error handling scenarios [It] should handle missing managed cluster during deployment
- /go/src/github.com/stolostron/multicluster-global-hub/test/integration/agent/migration/migration_from_syncer_test.go:510
- ref: https://storage.googleapis.com/test-platform-results/pr-logs/pull/stolostron_multicluster-global-hub/1888/pull-ci-stolostron-multicluster-global-hub-main-test-integration/1957235419108085760/build-log.txt

2. [FAIL] Migration Phase Transitions - Simplified [It] should complete full successful migration lifecycle
- /go/src/github.com/stolostron/multicluster-global-hub/test/integration/manager/migration/migration_phase_test.go:298 
- ref: https://storage.googleapis.com/test-platform-results/pr-logs/pull/stolostron_multicluster-global-hub/1887/pull-ci-stolostron-multicluster-global-hub-main-test-integration/1957235370340913152/build-log.txt

# How to do
1. these are flaky errors
2. fetch the referenced links to see the actual test failures and identify the root cause
3. if Claude Code is unable to fetch from storage.googleapis.com, you can use curl to download and then read it.
4. if check this folder ../../stolostron/multicluster-global-hub/test, use ./test instead
5. before fix the code, tell me the root cause of the failures

# How to verfiy
- resolve the compliation errors
- run `make integration-test` to verify
