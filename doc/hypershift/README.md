# Multicluster Global Hub Manages Lifecycle of Red Hat Advanced Cluster Management Hub Clusters Using HyperShift Control Plane

Multicluster global hub supports Red Hat Advanced Cluster Management for Kubernetes hub clusters from instantiation to ensuring the health after it is running. This document explains how multicluster global hub manages the lifecycle of Red Hat Advanced Cluster Management hub clusters that are using HyperShift control-plane. With HyperShift and multicluster global hub, multiple hosted clusters can be provisioned by one HyperShift instance and enabled as the regional hubs by multicluster global hub controller.

**Note:** This guide is used to enable the regional hub from HyperShift hosted cluster with zero work nodes. If you want to provision one of more worker nodes with HyperShift hosted cluster, see the [HyperShift official document](https://hypershift-docs.netlify.app/).

## Prerequisites

1. Two Red Hat OpenShift Container Platform Clusters on version 4.10 or later. One is the multicluster global hub cluster, and the other is the HyperShift management cluster.
2. Red Hat Advanced Cluster Management version 2.5 or later, which is installed on the first OpenShift Container Platform cluster from the Operator Hub. (Alternate: https://github.com/stolostron/deploy).
3. If the second OpenShift Container Platform cluster (which is the HyperShift management cluster) is deployed on bare metal and the HyperShift hosted clusters are also provisioned on bare metal, make sure worker nodes of the second OpenShift Container Platform cluster have external IP addresses. If they do not have external IP addresses, the API service of the HyperShift hosted cluster cannot be externally accessed.
4. If the second OpenShift Container Platform cluster (which is the HyperShift management cluster) is deployed on AWS and HyperShift hosted clusters are also provisioned on AWS, a Route53 public zone for cluster DNS records should be ready on AWS.

    To create a public zone:

    ```
    BASE_DOMAIN=www.example.com
    aws route53 create-hosted-zone --name $BASE_DOMAIN --caller-reference $(whoami)-$(date --rfc-3339=date)
    ```

5. A valid [pull secret](https://cloud.redhat.com/openshift/install/aws/installer-provisioned) file for the `quay.io/openshift-release-dev` repository.
6. An [AWS credentials file](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html) with permissions to create infrastructure for the cluster.
7. An S3 bucket with public access to host OIDC discovery documents for the HyperShift hosted clusters. This is required for both the HyperShift hosted cluster on bare metal and AWS.

    To create the bucket (in `us-east-1`):

    ```
    BUCKET_NAME=<S3-bucket-used-for-OIDC-provider>
    aws s3api create-bucket --acl public-read --bucket ${BUCKET_NAME}
    ```

    To create the bucket in a region other than `us-east-1`:

    ```
    BUCKET_NAME=<S3-bucket-used-for-OIDC-provider>
    REGION=us-east-2
    aws s3api create-bucket --acl public-read --bucket ${BUCKET_NAME} \
        --create-bucket-configuration LocationConstraint=$REGION \
        --region ${REGION}
    ```

## Get Started

1. Follow the [guide](https://github.com/stolostron/multicluster-global-hub/tree/release-2.5/deploy) to install HoH(`> v0.4.0` or `latest`) to Red Hat Advanced Cluster Management hub cluster.
2. In the Multicluster Global Hub console, import the second OpenShift Container Platform cluster as a regional hub (with name `hypermgt` for simplicity) in which hypershift operator will be running.
3. Set the following environment variable to use throughout the guide:

```
export HYPERSHIFT_MGMT_CLUSTER=hypermgt
```

4. Enable the hypershift-addon in the Multicluster Global Hub:

   - Patch the multicluster engine to enable HyperShift:

    ```
    oc patch multiclusterengine multiclusterengine --type=merge -p '{"spec":{"overrides":{"components":[{"name":"hypershift-preview","enabled": true}]}}}'
    ```

    - Wait for the `hypershift-addon-manager` and `hypershift-deployment-controller` are up and running:

    ```
    # oc -n multicluster-engine get pod -l app=hypershift-addon-manager
    NAME                                        READY   STATUS    RESTARTS   AGE
    hypershift-addon-manager-6567c47df5-7h2dv   1/1     Running   0          1m
    # oc -n multicluster-engine get pod -l name=hypershift-deployment-controller
    NAME                                                READY   STATUS    RESTARTS   AGE
    hypershift-deployment-controller-7d578ccbd8-hc7m4   1/1     Running   0          1m
    ```

    - Install `hypershift-addon` for the HyperShift managed cluster:

    ```
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

    - Create AWS S3 credentials for `hypershift` operator:

    ```
    AWS_S3_CREDS=<aws-credential-file>
    BUCKET_NAME=<S3-bucket-used-for-OIDC-provider>
    BUCKET_REGION=<aws-bucket-region>
    oc -n ${HYPERSHIFT_MGMT_CLUSTER} create secret generic \
        hypershift-operator-oidc-provider-s3-credentials \
        --from-file=credentials=${AWS_S3_CREDS} \
        --from-literal=bucket=${BUCKET_NAME} \
        --from-literal=region=${BUCKET_REGION}
    ```

    - Wait until the `hypershift-addon` is available:

    ```
    oc wait --for=condition=Available managedclusteraddon/hypershift-addon -n ${HYPERSHIFT_MGMT_CLUSTER} --timeout=600s
    ```

5. Create the HyperShift hosted cluster and enable it as a regional hub.

    - Refer to the [guide](./hypershift-aws.md) for HyperShift management cluster and hosted cluster provisioned on AWS.
    - Refer to the [guide](./hypershift-bm.md) for HyperShift management cluster and hosted cluster provisioned on bare metal.

6. Import a managed cluster to the HyperShift regional hub by following the [guide](./hypershift-regionalhub-import-cluster.md).
