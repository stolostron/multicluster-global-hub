#!/usr/bin/env bash

binDir="/usr/bin"

function versionCompare() {
  if [[ $1 == $2 ]]
  then
    return 0
  fi
  local IFS=.
  local i version1=($1) version2=($2)
  # fill empty fields in version1 with zeros
  for ((i=${#version1[@]}; i<${#version2[@]}; i++))
  do
    version1[i]=0
  done
  for ((i=0; i<${#version1[@]}; i++))
  do
    if [[ -z ${version2[i]} ]]
    then
      # fill empty fields in version2 with zeros
      version2[i]=0
    fi
    if ((10#${version1[i]} > 10#${version2[i]}))
    then
      return 1
    fi
    if ((10#${version1[i]} < 10#${version2[i]}))
    then
      return 2
    fi
  done

  return 0
}

function checkGolang() {
  export PATH=$PATH:/usr/local/go/bin
  if ! command -v go >/dev/null 2>&1; then
    wget https://dl.google.com/go/go1.19.8.linux-amd64.tar.gz >/dev/null 2>&1
    sudo tar -C /usr/local/ -xvf go1.19.8.linux-amd64.tar.gz >/dev/null 2>&1
    sudo rm go1.18.4.linux-amd64.tar.gz
  fi
  if [[ $(go version) < "go version go1.19" ]]; then
    echo "go version is less than 1.19, update to 1.19"
    sudo rm -rf /usr/local/go
    wget https://dl.google.com/go/go1.19.8.linux-amd64.tar.gz >/dev/null 2>&1
    sudo tar -C /usr/local/ -xvf go1.19.8.linux-amd64.tar.gz >/dev/null 2>&1
    sudo rm go1.19.8.linux-amd64.tar.gz
    sleep 2
  fi
  echo "go version: $(go version)"
}

function checkKubectl() {
  if ! command -v kubectl >/dev/null 2>&1; then 
    echo "This script will install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) on your machine"
    if [[ "$(uname)" == "Linux" ]]; then
        curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/linux/amd64/kubectl
    elif [[ "$(uname)" == "Darwin" ]]; then
        curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.21.0/bin/darwin/amd64/kubectl
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
    source ./install.sh 0.4.1
    rm ./install.sh
  fi
  echo "clusteradm path: $(which clusteradm)"
}

function checkKind() {
  if ! command -v kind >/dev/null 2>&1; then 
    curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.14.0/kind-linux-amd64 
    chmod +x ./kind
    sudo mv ./kind ${binDir}/kind
  fi
  if [[ $(kind version |awk '{print $2}') < "v0.12.0" ]]; then
    sudo rm -rf $(which kind)
    curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.14.0/kind-linux-amd64 
    chmod +x ./kind
    sudo mv ./kind ${binDir}/kind
  fi
  echo "kind version: $(kind version)"
}

function checkGinkgo() {
  if ! command -v ginkgo >/dev/null 2>&1; then 
    go install github.com/onsi/ginkgo/v2/ginkgo@v2.9.2
    go get github.com/onsi/gomega/...
    sudo mv $(go env GOPATH)/bin/ginkgo ${binDir}/ginkgo
  fi 
  echo "ginkgo version: $(ginkgo version)"
}

function installDocker() {
  sudo yum install -y yum-utils device-mapper-persistent-data lvm2
  sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
  sudo yum install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

  sleep 5
  sudo systemctl start docker
  sudo systemctl enable docker
}

function checkDocker() {
  if ! command -v docker >/dev/null 2>&1; then 
    installDocker
  fi
  versionCompare $(docker version --format '{{.Client.Version}}') "20.10.0"
  verCom=$?
  if [ $verCom -eq 2 ]; then
    # upgrade
    echo "remove old version of docker $(docker version --format '{{.Client.Version}}')"
    sudo yum remove -y docker docker-client docker-client-latest docker-common docker-latest docker-latest-logrotate docker-logrotate docker-selinux  docker-engine-selinux docker-engine 
    installDocker
  fi
  echo "docker version: $(docker version --format '{{.Client.Version}}')"
}

function checkVolume() {
  if [[ $(df -h | grep -v Size | awk '{print $2}' | sed -e 's/G//g' | awk 'BEGIN{ max = 0 } {if ($1 > max) max = $1; fi} END{print max}') -lt 80 ]]; then
    maxSize=$(lsblk | awk '{print $1,$4}' | grep -v Size | grep -v M | sed -e 's/G//g' | awk 'BEGIN{ max = 0 } {if ($2 > max) max = $2; fi} END{print max}')
    mountName=$(lsblk | grep "$maxSize"G | awk '{print $1}')
    echo "mounting /dev/${mountName}: ${maxSize}"
    sudo mkfs -t xfs /dev/${mountName}        
    sudo mkdir -p /data/docker                    
    sudo mount /dev/${mountName} /data/docker  

    sudo systemctl stop docker.socket
    sudo systemctl stop docker
    sudo systemctl stop containerd

    sudo mv /var/lib/docker /data/docker
    sudo sed -i "s/ExecStart=\/usr\/bin\/dockerd\ -H/ExecStart=\/usr\/bin\/dockerd\ -g\ \/data\/docker\ -H/g" /lib/systemd/system/docker.service
    sleep 2
    
    sudo systemctl daemon-reload
    sudo systemctl start docker
    sudo systemctl enable docker
  fi
  echo "docker root dir: $(docker info -f '{{ .DockerRootDir}}')"
}

checkGolang
checkDocker
if [[ $OPENSHIFT_CI == "true" ]]; then 
  checkVolume
fi
checkKind
checkKubectl
checkClusteradm
checkGinkgo
