#!/bin/bash

KUBECONFIG=${1:-$KUBECONFIG}
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

NS=olm
kubectl create namespace $NS --dry-run=client -o yaml | kubectl apply -f -
csvPhase=$(kubectl get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
if [[ "$csvPhase" == "Succeeded" ]]; then
  echo "OLM is already installed in ${NS} namespace. Exiting..."
  exit 1
fi
  
GIT_PATH="https://raw.githubusercontent.com/operator-framework/operator-lifecycle-manager/v0.21.2"
kubectl apply -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml"
kubectl wait --for=condition=Established -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml"
sleep 10
kubectl apply -f "${GIT_PATH}/deploy/upstream/quickstart/olm.yaml"

retries=30
until [[ $retries == 0 ]]; do
  csvPhase=$(kubectl get csv -n "${NS}" packageserver -o jsonpath='{.status.phase}' 2>/dev/null || echo "Waiting for CSV to appear")
  if [[ "$csvPhase" == "Succeeded" ]]; then
    break
  fi
  kubectl apply -f "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml"
  kubectl apply -f "${GIT_PATH}/deploy/upstream/quickstart/olm.yaml"
  echo "${GIT_PATH}/deploy/upstream/quickstart/crds.yaml"
  echo "${GIT_PATH}/deploy/upstream/quickstart/olm.yaml"
  sleep 10
  retries=$((retries - 1))
done
kubectl rollout status -w deployment/packageserver --namespace="${NS}" 

if [ $retries == 0 ]; then
  echo "CSV \"packageserver\" failed to reach phase succeeded"
  exit 1
fi
echo "CSV \"packageserver\" install succeeded"