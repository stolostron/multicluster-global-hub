#!/usr/bin/env bash

binDir="/usr/bin"

function checkGolang() {
  if ! command -v go >/dev/null 2>&1; then
    wget https://dl.google.com/go/go1.17.7.linux-amd64.tar.gz
    tar -C /usr/local/ -xvf go1.17.7.linux-amd64.tar.gz >/dev/null 2>&1
  fi
  if [[ $(go version) < "go version go1.17" ]]; then
    echo "go version is less than 1.17, update to 1.17"
    sudo rm -rf /usr/local/go
    sudo wget https://dl.google.com/go/go1.17.7.linux-amd64.tar.gz
    sudo tar -C /usr/local/ -xvf go1.17.7.linux-amd64.tar.gz >/dev/null 2>&1
  fi
  echo "go version: $(go version)"
}

function checkKubectl() {
  if ! command -v kubectl >/dev/null 2>&1; then 
    echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
        curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.18.0/bin/linux/amd64/kubectl
    elif [[ "$(uname)" == "Darwin" ]]; then
        curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.18.0/bin/darwin/amd64/kubectl
    fi
    chmod +x ./kubectl
    sudo mv ./kubectl ${binDir}/kubectl
  fi
  echo "kubectl version: $(kubectl version --client --short)"
}

function checkClusteradm() {
  if ! command -v clusteradm >/dev/null 2>&1; then 
    # curl -L https://raw.githubusercontent.com/open-cluster-management-io/clusteradm/main/install.sh | bash 
    curl -LO https://raw.githubusercontent.com/open-cluster-management-io/clusteradm/main/install.sh
    chmod +x ./install.sh
    INSTALL_DIR=$binDir
    source ./install.sh
    rm ./install.sh
  fi
  echo "clusteradm path: $(which clusteradm)"
}

function checkKind() {
  if ! command -v kind >/dev/null 2>&1; then 
    echo "This script will install kind (https://kind.sigs.k8s.io/) on your machine."
    curl -Lo ./kind-amd64 "https://kind.sigs.k8s.io/dl/v0.12.0/kind-$(uname)-amd64"
    chmod +x ./kind-amd64
    sudo mv ./kind-amd64 ${binDir}/kind
  fi
  echo "kind version: $(kind version)"
}

function checkGinkgo() {
  if ! command -v ginkgo >/dev/null 2>&1; then 
    go install github.com/onsi/ginkgo/v2/ginkgo@latest
    go get github.com/onsi/gomega/...
    mv /root/go/bin/ginkgo $binDir         # move ginkgo to the bin path
  fi 
  echo "ginkgo version: $(ginkgo version)"
}

checkGolang
checkKind
checkKubectl
checkClusteradm
checkGinkgo