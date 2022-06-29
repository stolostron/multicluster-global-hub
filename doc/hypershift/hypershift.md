# Enable HyperShift Hosted Cluster as Leaf Hub

This guide is used to enable leaf hub from HyperShift hosted cluster w/ zero work node.

## Prerequisites

1. Two OpenShift Cluster, version 4.10+ is recommended, one will act as hub of hubs cluster, another will be HyperShift management cluster.
2. ACM 2.5+ installed on the first cluster from Operator Hub. (Alternate: https://github.com/stolostron/deploy)

### Get Started

1. Follow the [guide](https://github.com/stolostron/hub-of-hubs/tree/release-2.5/deploy) to install HoH to ACM hub cluster.
3. In the HoH console, import the second cluster as leaf hub (with name "hypermgt") in which hypershift operator will be running.
4. Follow the [guide](./hypershift-install.md) enable hypershift-addon in HoH and create hosted cluster w/ zero worker node (it will be auto imported as managed cluster).
5. Enable the ACM addons(policy and application) for the hypershift hosted cluster running in hosted mode:

```bash
export HYPERSHIFT_MANAGED_CLUSTER_NAME=$(oc get managedcluster -l hub-of-hubs.open-cluster-management.io/created-by-hypershift=true -o jsonpath='{.items[0].metadata.name}')
oc -n hypermgt patch manifestwork ${HYPERSHIFT_MANAGED_CLUSTER_NAME}-hosted-klusterlet --type=json \
    -p='[{"op":"replace","path":"/spec/workload/manifests/1/spec/registrationImagePullSpec","value":"quay.io/morvencao/registration:latest"}]'
envsubst < ./manifestwork-policy-framework.yaml | oc apply -f -
envsubst < ./manifestwork-config-policy-controller.yaml | oc apply -f -
envsubst < ./application-managedclusteraddon.yml | oc apply -f -
envsubst < ./manifestwork-application-manager.yaml | oc apply -f -
```

Workaround for the permission of application addon:

```bash
oc --kubeconfig=<kubeconfig-path-to-hypershift-management-cluster> adm policy add-scc-to-user \
    anyuid system:serviceaccount:klusterlet-${HYPERSHIFT_MANAGED_CLUSTER_NAME}:application-manager
oc --kubeconfig=<kubeconfig-path-to-hypershift-management-cluster> -n klusterlet-${HYPERSHIFT_MANAGED_CLUSTER_NAME} \
    delete rs -l component=application-manager
# create namespace in hypershift hosted cluster for leader election
oc --kubeconfig=<kubeconfig-path-to-hypershift-hosted-cluster> create ns klusterlet-${HYPERSHIFT_MANAGED_CLUSTER_NAME}
```

7. Follow the [guide](./import-cluster-to-hypershift-leafhub.md) to import a managed cluster to the HyperShift leaf hub.
