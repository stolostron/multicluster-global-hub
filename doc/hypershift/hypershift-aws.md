# Provision HyperShift Hosted Cluster on AWS

This document is used to provision HyperShift hosted cluster on AWS platform with hypershift-addon in ACM 2.5+.

## Prerequisites

1. Set the following environment variables that will be used later:

```bash
export AWS_ACCESS_KEY_ID=<aws-access-key-id>
export AWS_SECRET_ACCESS_KEY=<aws-secret-access-key>
export INFRA_REGION=<cloud-provider-region>
export BASE_DOMAIN=<aws-domain>
export OPENSHIFT_PULL_SECRET_FILE=<openshift-pull-secret-file>
export SSH_PRIVATE_KEY_FILE=<ssh-private-key-file>
export SSH_PUBLIC_KEY_FILE=<ssh-public-key-file>
export CLOUD_PROVIDER_SECRET_NAME=<cloud-provider-secret-name>
export CLOUD_PROVIDER_SECRET_NAMESPACE=<namespace-for-cloud-provider-secret>
```

2. Create AWS cloud provider `Credential` in a project from ACM UI(https://console-openshift-console.apps.<openshift-domain>/multicloud/credentials) or by the following command:

```bash
oc create ns ${CLOUD_PROVIDER_SECRET_NAMESPACE}
oc create secret generic ${CLOUD_PROVIDER_SECRET_NAME} \
  -n ${CLOUD_PROVIDER_SECRET_NAMESPACE} \
  --from-literal=aws_access_key_id=${AWS_ACCESS_KEY_ID} \
  --from-literal=aws_secret_access_key=${AWS_SECRET_ACCESS_KEY} \
  --from-literal=baseDomain=${BASE_DOMAIN} \
  --from-literal=httpProxy="" \
  --from-literal=httpsProxy="" \
  --from-literal=noProxy="" \
  --from-file=pullSecret=${OPENSHIFT_PULL_SECRET_FILE} \
  --from-file=ssh-privatekey=${SSH_PRIVATE_KEY_FILE} \
  --from-file=ssh-publickey=${SSH_PUBLIC_KEY_FILE} \
  --from-literal=additionalTrustBundle=""
```

## Create HyperShift Hosted Cluster on AWS

1. Set the following environment variables for `HypershiftDeployment` resource:

```bash
export HYPERSHIFT_MGMT_CLUSTER=hypermgt
export HYPERSHIFT_HOSTING_NAMESPACE=clusters
export HYPERSHIFT_DEPLOYMENT_NAME=<hypershiftdeployment-name>
export OPENSHIFT_RELEASE_IMAGE=quay.io/openshift-release-dev/ocp-release:4.11.0-fc.3-x86_64
```

2. Create `HypershiftDeployment` resource to provision AWS hosted cluster:

```bash
oc apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: HypershiftDeployment
metadata:
  name: ${HYPERSHIFT_DEPLOYMENT_NAME}
  namespace: ${CLOUD_PROVIDER_SECRET_NAMESPACE}
spec:
  hostingCluster: ${HYPERSHIFT_MGMT_CLUSTER}
  hostingNamespace: ${HYPERSHIFT_HOSTING_NAMESPACE}
  infrastructure:
    cloudProvider:
      name: ${CLOUD_PROVIDER_SECRET_NAME}
    configure: true
    platform:
      aws:
        region: ${INFRA_REGION}
  nodePools:
  - name: ${HYPERSHIFT_DEPLOYMENT_NAME}-workers
    spec:
      clusterName: ${HYPERSHIFT_DEPLOYMENT_NAME}
      management:
        upgradeType: Replace
      nodeCount: 0
      platform:
        type: AWS
      release:
        image: ${OPENSHIFT_RELEASE_IMAGE}
EOF
```

3. Get managed cluster name of the hypershift hosted cluster created in last step and wait until managed cluster is available:

```bash
export HYPERSHIFT_MANAGED_CLUSTER_NAME=$(oc get managedcluster | grep ${HYPERSHIFT_DEPLOYMENT_NAME} | awk '{print $1}')
oc wait --for=condition=ManagedClusterConditionAvailable managedcluster/${HYPERSHIFT_MANAGED_CLUSTER_NAME} --timeout=600s
```

4. Get kubeconfig for the HyperShift hosted cluster:

```bash
oc -n ${HYPERSHIFT_MGMT_CLUSTER} get secret ${HYPERSHIFT_MANAGED_CLUSTER_NAME}-admin-kubeconfig -o jsonpath="{.data.kubeconfig}" | base64 -d > <kubeconfig-path-to-hypershift-hosted-cluster>
```

5. Enable the ACM addons(policy and application in hosted mode) for the hypershift hosted cluster:

```bash
envsubst < ./manifests/managedclusteraddon-application.yaml | oc apply -f -
oc -n ${HYPERSHIFT_MGMT_CLUSTER} patch manifestwork ${HYPERSHIFT_MANAGED_CLUSTER_NAME}-hosted-klusterlet --type=json \
    -p='[{"op":"replace","path":"/spec/workload/manifests/1/spec/registrationImagePullSpec","value":"quay.io/morvencao/registration:latest"}]'
envsubst < ./manifests/manifestwork-policy-framework.yaml | oc apply -f -
envsubst < ./manifests/manifestwork-config-policy-controller.yaml | oc apply -f -
envsubst < ./manifests/manifestwork-application-manager.yaml | oc apply -f -
```

  _Note:_ The application addon in HyperShift management cluster fails to start due to permission issue, the workaround is logging into the HyperShift management cluster and executing the following command:

  ```bash
  oc --kubeconfig=<kubeconfig-path-to-hypershift-management-cluster> adm policy add-scc-to-user \
    anyuid system:serviceaccount:klusterlet-${HYPERSHIFT_MANAGED_CLUSTER_NAME}:application-manager
  oc --kubeconfig=<kubeconfig-path-to-hypershift-management-cluster> -n klusterlet-${HYPERSHIFT_MANAGED_CLUSTER_NAME} \
    delete rs -l component=application-manager
  # create namespace in hypershift hosted cluster for leader election
  oc --kubeconfig=<kubeconfig-path-to-hypershift-hosted-cluster> create ns klusterlet-${HYPERSHIFT_MANAGED_CLUSTER_NAME}
  ```

6. Check the ACM addons(policy and application) are available:

```bash
oc wait --for=condition=Available managedclusteraddon/application-manager -n ${HYPERSHIFT_MANAGED_CLUSTER_NAME} --timeout=600s
oc wait --for=condition=Available managedclusteraddon/config-policy-controller -n ${HYPERSHIFT_MANAGED_CLUSTER_NAME} --timeout=600s
oc wait --for=condition=Available managedclusteraddon/governance-policy-framework -n ${HYPERSHIFT_MANAGED_CLUSTER_NAME} --timeout=600s
```