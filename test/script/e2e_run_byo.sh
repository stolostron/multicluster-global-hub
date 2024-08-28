# CleanUp globalhub 
bash "$CURRENT_DIR/e2e_clean_globalhub.sh"

wait
# async middlewares
bash "$CURRENT_DIR/e2e_postgres.sh" "$CONFIG_DIR/hub1" "$GH_KUBECONFIG" 2>&1 & # install postgres into hub1
echo "$!" >"$CONFIG_DIR/PID"

bash "$CURRENT_DIR/e2e_kafka.sh" "$CONFIG_DIR/hub2" "$GH_KUBECONFIG" 2>&1 &
echo "$!" >>"$CONFIG_DIR/PID"

## run e2e
bash $CURRENT_DIR/e2e_run.sh -f "e2e-test-localpolicy,e2e-test-placement,e2e-test-app,e2e-test-policy,e2e-tests-backup,e2e-test-grafana" -v $(VERBOSE)
