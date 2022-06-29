# Enable HyperShift in ACM and Provision Hosted Cluster

Ref: https://github.com/stolostron/hypershift-deployment-controller/tree/main/samples/quickstart

## Prerequisites

1. Install ACM 2.5+
2. Set the following environment that will be used later: 

```bash
export AWS_S3_CREDS=<aws-credential>
export BUCKET_NAME=<S3-bucket-used-for-OIDC-provider>
export BUCKET_REGION=<aws-bucket-region>
```

3. Create the S3 bucket from command line if it doesn't exist yet:

```bash
aws s3api create-bucket --acl public-read --bucket ${BUCKET_NAME}
```

4. Make sure the S3 bucket can be public accessed:

```bash
curl https://${BUCKET_NAME}.s3.${BUCKET_REGION}.amazonaws.com 
```

## Enable HyperShift Addon

1. Patch the MCE to enable the HyperShift:

```bash
oc patch multiclusterengine multiclusterengine --type=merge -p '{"spec":{"overrides":{"components":[{"name":"hypershift-preview","enabled": true}]}}}'
```

2. Wait for the `hypershift-addon-manager` and `hypershift-deployment-controller` are up and running:

```bash
# oc -n multicluster-engine get pod -l app=hypershift-addon-manager
NAME                                        READY   STATUS    RESTARTS   AGE
hypershift-addon-manager-6567c47df5-7h2dv   1/1     Running   0          1m
# oc -n multicluster-engine get pod -l name=hypershift-deployment-controller
NAME                                                READY   STATUS    RESTARTS   AGE
hypershift-deployment-controller-7d578ccbd8-hc7m4   1/1     Running   0          1m
```

3. Install hypershift addon for managed cluster named `hypermgt`:

```bash
oc apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: hypershift-addon
  namespace: hypermgt
spec:
  installNamespace: open-cluster-management-agent-addon
EOF
```

4. Create AWS S3 credentials for hypershift-operator:

```bash
oc -n hypermgt create secret generic hypershift-operator-oidc-provider-s3-credentials \
  --from-file=credentials=${AWS_S3_CREDS} \
  --from-literal=bucket=${BUCKET_NAME} \
  --from-literal=region=${BUCKET_REGION}
```

5. Wait until the hypershift-addon is available:

```bash
oc wait --for=condition=Available managedclusteraddon/hypershift-addon -n hypermgt --timeout=600s
```

## Create HyperShift Hosted Cluster with HyperShift Addon

1. Set the following environment that will be used later:

```bash
export CLOUD_PROVIDER_SECRET_NAMESPACE=<namespace-for-cloud-provider-secret>
export AWS_ACCESS_KEY_ID=<aws-access-key-id>
export AWS_SECRET_ACCESS_KEY=<aws-secret-access-key>
export BASE_DOMAIN=<aws-domain>
export OPENSHIFT_PULL_SECRET_FILE=<openshift-pull-secret-file>
export SSH_PRIVATE_KEY_FILE=<ssh-private-key-file>
export SSH_PUBLIC_KEY_FILE=<ssh-public-key-file>
export CLOUD_PROVIDER_SECRET_NAME=<cloud-provider-secret-name>
export INFRA_REGION=<cloud-provider-region>
export HYPERSHIFT_DEPLOYMENT_NAME=<hypershiftdeployment-name>
```

2. Create a Cloud Provider `Credential` in a project from ACM UI(https://console-openshift-console.apps.<your-openshift-domain>/multicloud/credentials) or with the following commands:

```bash
oc create ns ${CLOUD_PROVIDER_SECRET_NAMESPACE}
oc -n ${CLOUD_PROVIDER_SECRET_NAMESPACE} create secret generic ${CLOUD_PROVIDER_SECRET_NAME} \
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

3. Create a `HypershiftDeployment` instance to provision AWS hosted cluster:

```bash
oc apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: HypershiftDeployment
metadata:
  name: ${HYPERSHIFT_DEPLOYMENT_NAME}
  namespace: ${CLOUD_PROVIDER_SECRET_NAMESPACE}
spec:
  hostingCluster: hypermgt
  hostingNamespace: hsclusters
  infrastructure:
    cloudProvider:
      name: ${CLOUD_PROVIDER_SECRET_NAME}
    configure: True
    platform:
      aws:
        region: ${INFRA_REGION}
  nodePools:
  - name: hypershift-nodepool
    spec:
      clusterName: hypershift-nodepool
      management:
        upgradeType: Replace
      nodeCount: 0
      platform:
        type: AWS
      release:
        image: quay.io/openshift-release-dev/ocp-release:4.11.0-fc.3-x86_64
EOF
```

4. Get the managed cluster name of the hypershift hosted cluster created in last step:

```bash
export HYPERSHIFT_MANAGED_CLUSTER_NAME=$(oc get managedcluster | grep ${HYPERSHIFT_DEPLOYMENT_NAME} | awk '{print $1}')
```

5. Wait until the managed cluster is available:

```bash
oc wait --for=condition=ManagedClusterConditionAvailable managedcluster/${HYPERSHIFT_MANAGED_CLUSTER_NAME} --timeout=600s
```
