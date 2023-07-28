#!/bin/bash

clusterName=$1
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
echo "currentDir: $currentDir"
if [[ -z "$(docker ps | grep "${clusterName}" || true)" ]]; then
  echo "Creating microshift cluster ${clusterName}..."
  # 9095 kafka, 32432 postgres
  docker run -d --rm --name $clusterName --privileged -v hub-data:/var/lib -p 127.0.0.1:6443:6443 -p 127.0.0.1:30080:8080 -p 30095:9095 -p 32432:32432 quay.io/microshift/microshift-aio:latest
  until docker cp $clusterName:/var/lib/microshift/resources/kubeadmin/kubeconfig $currentDir/../config/kubeconfig-microshift > /dev/null 2>&1
  do
    echo "Waiting for microshift kubeconfig is ready..."
    sleep 1
  done
  echo "containerIp=$(docker inspect -f '{{.NetworkSettings.IPAddress}}' $clusterName)"
  containerIp=$(docker inspect -f '{{.NetworkSettings.IPAddress}}' $clusterName)
  sed "s;127.0.0.1;${containerIp};g" $currentDir/../config/kubeconfig-microshift > $KUBECONFIG
fi 
kubectl config use-context "microshift"
SECOND=0
while [[ -z $(kubectl get deploy router-default -n openshift-ingress --ignore-not-found) ]]; do
  if [ $SECOND -gt 100 ]; then
    echo "Timeout waiting for deploying router-default $secret"
    exit 1
  fi
  echo "Waiting for router-default to be created..."
  sleep 1;
  (( SECOND = SECOND + 1 ))
done;
kubectl wait deployment -n openshift-ingress router-default --for condition=Available=True --timeout=200s
