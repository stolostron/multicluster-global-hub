# Hub-of-Hubs Manages Lifecycle of ACM Hub Clusters Using HyperShift Control Plane

Hub-of-Hubs supports ACM hub clusters from instantiation to ensuring the health. This document explain how Hub-of-Hubs manages lifecycle of ACM hub clusters which are using HyperShift control-plane. With HyperShift and Hub-of-Hubs, multiple hosted clusters can be provisioned by one HyperShift and enabled as leaf hubs by Hub-of-Hubs controller.

_Note_: This guide is used to enable leaf hub from HyperShift hosted cluster w/ zero work node, if you're trying to provisioned 1+ worker node to HyperShift hosted cluster, please refer to the [HyperShift official document](https://hypershift-docs.netlify.app/).

## Prerequisites

1. Two OpenShift Cluster with recommended version 4.10+, one will be hub-of-hubs cluster, another will be HyperShift management cluster.
2. ACM 2.5+ installed on the first OpenShift cluster from Operator Hub. (Alternate: https://github.com/stolostron/deploy)
3. If the second OpenShift cluster (which will be HyperShift management cluster) is deployed on bare metal and HyperShift hosted clusters are also provisioned on bare metal, make sure worker nodes of the second OpenShift cluster have external IPs, othertwise, API service of the HyperShift hosted cluster can't be accessed from external.
4. If the second OpenShift cluster (which will be HyperShift management cluster) is deployed on AWS and HyperShift hosted clusters are also provisioned on AWS, a Route53 public zone for cluster DNS records should be ready on AWS.

    To create a public zone:

    ```bash
    BASE_DOMAIN=www.example.com
    aws route53 create-hosted-zone --name $BASE_DOMAIN --caller-reference $(whoami)-$(date --rfc-3339=date)
    ```

5. A valid [pull secret](https://cloud.redhat.com/openshift/install/aws/installer-provisioned) file for the `quay.io/openshift-release-dev` repository.
6. An [AWS credentials file](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html) with permissions to create infrastructure for the cluster.
7. An S3 bucket with public access to host OIDC discovery documents for the HyperShift hosted clusters. (This is required for both the HyperShift hosted cluster on bare metal and AWS)

    To create the bucket (in `us-east-1`):

    ```bash
    BUCKET_NAME=<S3-bucket-used-for-OIDC-provider>
    aws s3api create-bucket --acl public-read --bucket ${BUCKET_NAME}
    ```

    To create the bucket in a region other than `us-east-1`:

    ```bash
    BUCKET_NAME=<S3-bucket-used-for-OIDC-provider>
    REGION=us-east-2
    aws s3api create-bucket --acl public-read --bucket ${BUCKET_NAME} \
        --create-bucket-configuration LocationConstraint=$REGION \
        --region ${REGION}
    ```

## Get Started

1. Follow the [guide](https://github.com/stolostron/hub-of-hubs/tree/release-2.5/deploy) to install HoH(`> v0.4.0` or `latest`) to ACM hub cluster.
2. In the HoH console, import the second OpenShift cluster as a leaf hub (with name `hypermgt` for simplicity) in which hypershift operator will be running.
3. Set the following environment variable that will be used throughout the guide:

```bash
export HYPERSHIFT_MGMT_CLUSTER=hypermgt
```

4. Enable the hypershift-addon in HoH:

   - Patch the MCE to enable the HyperShift:

    ```bash
    oc patch multiclusterengine multiclusterengine --type=merge -p '{"spec":{"overrides":{"components":[{"name":"hypershift-preview","enabled": true}]}}}'
    ```

    - Wait for the `hypershift-addon-manager` and `hypershift-deployment-controller` are up and running:

    ```bash
    # oc -n multicluster-engine get pod -l app=hypershift-addon-manager
    NAME                                        READY   STATUS    RESTARTS   AGE
    hypershift-addon-manager-6567c47df5-7h2dv   1/1     Running   0          1m
    # oc -n multicluster-engine get pod -l name=hypershift-deployment-controller
    NAME                                                READY   STATUS    RESTARTS   AGE
    hypershift-deployment-controller-7d578ccbd8-hc7m4   1/1     Running   0          1m
    ```

    - Install hypershift-addon for the HyperShift managed cluster:

    ```bash
    oc apply -f - <<EOF
    apiVersion: addon.open-cluster-management.io/v1alpha1
    kind: ManagedClusterAddOn
    metadata:
      name: hypershift-addon
      namespace: ${HYPERSHIFT_MGMT_CLUSTER}
    spec:
      installNamespace: open-cluster-management-agent-addon
    EOF
    ```

    - Create AWS S3 credentials for hypershift operator:

    ```bash
    AWS_S3_CREDS=<aws-credential-file>
    BUCKET_NAME=<S3-bucket-used-for-OIDC-provider>
    BUCKET_REGION=<aws-bucket-region>
    oc -n ${HYPERSHIFT_MGMT_CLUSTER} create secret generic \
        hypershift-operator-oidc-provider-s3-credentials \
        --from-file=credentials=${AWS_S3_CREDS} \
        --from-literal=bucket=${BUCKET_NAME} \
        --from-literal=region=${BUCKET_REGION}
    ```

    - Wait until the hypershift-addon is available:

    ```bash
    oc wait --for=condition=Available managedclusteraddon/hypershift-addon -n ${HYPERSHIFT_MGMT_CLUSTER} --timeout=600s
    ```

5. Create HyperShift hosted cluster and enable it as leaf hub.

    - Refer to the [guide](./hypershift-aws) for HyperShift management cluster and hosted cluster provisioned on AWS
    - Refer to the [guide](./hypershift-bm) for HyperShift management cluster and hosted cluster provisioned on bare metal

6. Import a managed cluster to the HyperShift leaf hub by following the [guide](./hypershift-leafhub-import-cluster.md).
