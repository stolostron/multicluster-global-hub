#!/bin/bash

clusterName=$1
currentDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
echo "currentDir: $currentDir"
if [[ -z "$(docker ps | grep "${clusterName}")" ]]; then
  echo "Creating microshift cluster ${clusterName}..."
  docker run -d --rm --name $clusterName --privileged -v hub-data:/var/lib -p 127.0.0.1:6443:6443 -p 127.0.0.1:30095:9095 -p 127.0.0.1:30080:8080 quay.io/microshift/microshift-aio:latest
  sleep 5
  until [ $(docker inspect -f {{.State.Running}} ${clusterName})=="true" ]; do
    echo "Waiting for $clusterName cluster to be ready..."
    sleep 5;
  done;
  docker cp $clusterName:/var/lib/microshift/resources/kubeadmin/kubeconfig $currentDir/../config/kubeconfig-microshift
  echo "containerIp=$(docker inspect -f '{{.NetworkSettings.IPAddress}}' $clusterName)"
  containerIp=$(docker inspect -f '{{.NetworkSettings.IPAddress}}' $clusterName)
  sed "s;127.0.0.1;${containerIp};g" $currentDir/../config/kubeconfig-microshift > $KUBECONFIG
fi 

kubectl wait deployment -n openshift-ingress router-default --for condition=Available=True --timeout=600s
kubectl wait deployment -n openshift-service-ca service-ca --for condition=Available=True --timeout=600s
