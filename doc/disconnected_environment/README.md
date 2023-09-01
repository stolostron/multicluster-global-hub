# Deploying Global Hub Operator in a disconnected environment 

In situations where a network connection is not available, you can deploy the Global Hub Operator in a disconnected environment.

## Prerequisites

- An image registry and a bastion host that have access to both the Internet and to your mirror registry
- Operator Lifecycle Manager ([OLM](https://docs.openshift.com/container-platform/4.11/operators/understanding/olm/olm-understanding-olm.html)) installed on your cluster
- Red Hat Advanced Cluster Management for Kubernetes version 2.7, or later, installed on your cluster
- A user account with `cluster-admin` permissions

## Mirror Registry

You must use a mirror image registry when installing Multicluster Global Hub in a disconnected environment. The image registry ensures that your clusters only use container images that satisfy your organizational controls on external content. You can complete the following two-step procedure to provision the mirror registry for global hub.
- [Creating a mirror registry](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.13/html/installing/disconnected-installation-mirroring#creating-mirror-registry)
- [Mirroring images for a disconnected installation](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.13/html/installing/disconnected-installation-mirroring#installing-mirroring-installation-images)

## Create an ImageContentSourcePolicy

You can configure an `ImageContentSourcePolicy` on your disconnected cluster to redirect image references to your mirror registry. This enables you to have your cluster obtain container images for the global hub operator on your mirror registry, rather than from the Internet-hosted registries. 

**Note**: The ImageContentSourcePolicy can only support the image mirror with image digest.

1. Create a file called `imagecontentsourcepolicy.yaml`:

    ```
    $ cat ./doc/disconnected_environment/imagecontentsourcepolicy.yaml
    ```

2. Add content that resembles the following content to the new file:

    ```
    apiVersion: operator.openshift.io/v1alpha1
    kind: ImageContentSourcePolicy
    metadata:
      name: global-hub-operator-icsp
    spec:
      repositoryDigestMirrors:
      - mirrors:
        - ${REGISTRY}//multicluster-globalhub
        source: registry.redhat.io/multicluster-globalhub
    ```
    
3. Apply `imagecontentsourcepolicy.yaml` by running the following command:

    ```
    envsubst < ./doc/disconnected-operator/imagecontentsourcepolicy.yaml | kubectl apply -f -
    ```

## Configure the image pull secret

If the Operator or Operand images that are referenced by a subscribed Operator require access to a private registry, you can either [provide access to all namespaces in the cluster, or to individual target tenant namespaces](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.13/html-single/operators/index#olm-creating-catalog-from-index_olm-managing-custom-catalogs). 

### Option 1. Configure the global hub image pull secret in an OpenShift cluster

**Note**: Applying the image pull secret on a pre-existing cluster causes a rolling restart of all of the nodes.

1. Export the user name from the pull secret:
    ```
    export USER=<the-registry-user>
    ```

2. Export the password from the pull secret:
    ```
    export PASSWORD=<the-registry-password>
    ```

3. Copy the pull secret:
    ```
    oc get secret/pull-secret -n openshift-config --template='{{index .data ".dockerconfigjson" | base64decode}}' > pull_secret.yaml
    ```

4. Log in using the pull secret:
    ```
    oc registry login --registry=${REGISTRY} --auth-basic="$USER:$PASSWORD" --to=pull_secret.yaml
    ```

5. Specify the global hub image pull secret:
    ```
    oc set data secret/pull-secret -n openshift-config --from-file=.dockerconfigjson=pull_secret.yaml
    ```

6. Remove the old pull secret:
    ```
    rm pull_secret.yaml
    ```

### Option 2. Configure image pull secret to an individual namespace

1. Create the secret in the tenant namespace by running the following command:
    ```
    oc create secret generic <secret_name> -n <tenant_namespace> \
    --from-file=.dockerconfigjson=<path/to/registry/credentials> \
    --type=kubernetes.io/dockerconfigjson
    ```

2. Link the secret to the service account for your operator/operand:
    ```
    oc secrets link <operator_sa> -n <tenant_namespace> <secret_name> --for=pull
    ```

## Add the GlobalHub operator catalog

### Build the GlobalHub catalog from upstream [Optional]

```bash
$ export REGISTRY=<operator-mirror-registry>
$ export VERSION=0.0.1
$ export IMAGE_TAG_BASE=${REGISTRY}/multicluster-global-hub-operator

$ cd ./operator
# update bundle
$ make generate manifests bundle
# build bundle image
$ make bundle-build bundle-push catalog-build catalog-push
$ cd ..
```

After running the above command, the following images will be built and pushed to the `$REGISTRY`.
- Bundle Image: `${REGISTRY}/multicluster-global-hub-operator-bundle:v0.0.1`
- Catalog Image: `${REGISTRY}/multicluster-global-hub-operator-catalog:v0.0.1`

### Create the CatalogSource object

refer [here](https://github.com/stolostron/multicluster-global-hub/tree/main/doc/disconnected_environment#configure-the-image-pull-secret) to prefer the imagepullsecret before starting create the catalogsource.

```bash
$ cat ./doc/disconnected_environment/catalogsource.yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: global-hub-operator-catalog
  namespace: openshift-marketplace
spec:
  displayName: global-hub-operator-catalog
  sourceType: grpc
  grpcPodConfig: {}
  secrets:
  - <global-hub-secret>
  image: ${REGISTRY}/multicluster-global-hub-operator-catalog:v${VERSION}
  publisher: global-hub-squad  

$ envsubst < ./doc/disconnected_environment/catalogsource.yaml | kubectl apply -f -
```

OLM polls catalog sources for available packages on a regular timed interval. After OLM polls the catalog source for your mirrored catalog, you can verify that the required packages are available from on your disconnected cluster by querying the available PackageManifest resources.

```bash
$ oc get packagemanifest multicluster-global-hub-operator
NAME                               CATALOG               AGE
multicluster-global-hub-operator   Community Operators   28m
```

## Install the Global Hub Operator

### Install the Operator from OperatorHub using the CLI

- Create the `OperatorGroup`
  
  Each namespace can have only one operator group. Replace `global-hub-operator-sdk-og` with the name of your operator group. and replace `open-cluster-management` namespace with your project namespace.

  ```bash
  $ cat ./doc/disconnected_environment/operatorgroup.yaml  
  apiVersion: operators.coreos.com/v1
  kind: OperatorGroup
  metadata:
    name: global-hub-operator-sdk-og
    namespace: multicluster-global-hub
  spec:
    targetNamespaces:
    - multicluster-global-hub

  $ oc apply -f ./doc/disconnected_environment/operatorgroup.yaml   
  ```
 
- Create the `Subscription`

  Replace the `multicluster-global-hub` namespace with your project namespace.
  
  ```bash
  $ cat ./doc/disconnected_environment/subscription.yaml
  apiVersion: operators.coreos.com/v1alpha1
  kind: Subscription
  metadata:
    name: multicluster-global-hub-operator
    namespace: multicluster-global-hub
  spec:
    channel: alpha
    installPlanApproval: Automatic
    name: multicluster-global-hub-operator
    source: global-hub-operator-catalog
    sourceNamespace: openshift-marketplace

  $ oc apply -f ./doc/disconnected_environment/subscription.yaml
  ```
  
- Check the global hub operator

  Replace the `multicluster-global-hub` namespace with your project namespace.

  ```bash
  $ oc get pods -n multicluster-global-hub
  NAME                                                READY   STATUS    RESTARTS   AGE
  multicluster-global-hub-operator-687584cb7c-fnftj   1/1     Running   0          2m12s
  
  $ oc describe pod -n multicluster-global-hub multicluster-global-hub-operator-687584cb7c-fnftj
  ...
  Events:
  Type    Reason          Age    From               Message
  ----    ------          ----   ----               -------
  Normal  Scheduled       2m52s  default-scheduler  Successfully assigned multicluster-global-hub/multicluster-global-hub-operator-5546668786-f7b7v to ip-10-0-137-91.ec2.internal
  Normal  AddedInterface  2m50s  multus             Add eth0 [10.128.1.7/23] from openshift-sdn
  Normal  Pulling         2m49s  kubelet            Pulling image "registry.redhat.io/multicluster-globalhub/multicluster-global-hub-operator@sha256:f385a9cfa78442526d6721fc7aa182ec6b98dffdabc78e2732bf9adbc5c8e0df"
  Normal  Pulled          2m35s  kubelet            Successfully pulled image "registry.redhat.io/multicluster-globalhub/multicluster-global-hub-operator@sha256:f385a9cfa78442526d6721fc7aa182ec6b98dffdabc78e2732bf9adbc5c8e0df" in 14.180033246s
  Normal  Created         2m35s  kubelet            Created container multicluster-global-hub-operator
  Normal  Started         2m35s  kubelet            Started container multicluster-global-hub-operator
  ...
  ```

### Install the Operator from OperatorHub using the web console
You can install and subscribe an Operator from OperatorHub using the OpenShift Container Platform web console. For more details, please refer [here](https://docs.openshift.com/container-platform/4.11/operators/admin/olm-adding-operators-to-cluster.html)

## Import the managed hub using customized image registry

### Configure the image registry annotations in MulticlusterGlobalHub CR

This is achieved by adding annotation to the MGH CR and specifying the image pull secret and image pull policy. e.g:
```yaml
apiVersion: operator.open-cluster-management.io/v1alpha4
kind: MulticlusterGlobalHub
metadata:
  annotations:
    mgh-image-repository: <private-image-registry>
  name: multiclusterglobalhub
  namespace: multicluster-global-hub
spec:
  imagePullPolicy: Always
  imagePullSecret: ecr-image-pull-secret
```
This was the global configuration and all of your managed hubs will use the same image registry and image pull secret.

To support different image registries for different managed hubs, we can use `ManagedClusterImageRegistry` API to import the managed hub.

### Config the ManagedClusterImageRegistry

refer [Importing a cluster that has a ManagedClusterImageRegistry](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.7/html-single/clusters/index#import-cluster-managedclusterimageregistry) to import the clusters using the `ManagedClusterImageRegistry` API.

- Create the placement/clusterset to select the target managed hub cluster, for example:
  ```yaml
  apiVersion: cluster.open-cluster-management.io/v1
  kind: ManagedCluster
  metadata:
    labels:
      cluster.open-cluster-management.io/clusterset: <cluster-set>
      vendor: auto-detect
      cloud: auto-detect
    name: <managed-hub>
  spec:
    hubAcceptsClient: true
    leaseDurationSeconds: 60
  ---
  apiVersion: cluster.open-cluster-management.io/v1beta2
  kind: ManagedClusterSet
  metadata:
    name: <cluster-set>
  ---
  apiVersion: cluster.open-cluster-management.io/v1beta2
  kind: ManagedClusterSetBinding
  metadata:
    name: <cluster-set>
    namespace: <placement-namespace>
  spec:
    clusterSet: <cluster-set>
  ---
  apiVersion: cluster.open-cluster-management.io/v1beta1
  kind: Placement
  metadata:
    name: <placement-name>
    namespace: <placement-namespace>
  spec:
    clusterSets:
      - <cluster-set>
    tolerations:
    - key: "cluster.open-cluster-management.io/unreachable"
      operator: Exists
  ```

- Create the `ManagedClusterImageRegistry` to replace the `Agent image`
  ```yaml
  apiVersion: imageregistry.open-cluster-management.io/v1alpha1
  kind: ManagedClusterImageRegistry
  metadata:
    name: <global-hub-cluster-image-registry>
    namespace: <placement-namespace>
  spec:
    placementRef:
      group: cluster.open-cluster-management.io
      resource: placements
      name: <placement-name>
    pullSecret:
      name: <image-pull-secret>
    registries:
      - mirror: <mirror-image-registry>
        source: <source-image-registry>
  ```

By configuring the above, a label and an annotation will be added to the selected `ManagedCluster`. This means that the agent image in the cluster will be replaced with the mirror image.
  - Label: `open-cluster-management.io/image-registry=<namespace.managedclusterimageregistry-name>`
  - Annotation: `open-cluster-management.io/image-registries: <image-registry-info>`

## References
- [Mirroring an Operator catalog](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-mirror-catalog_olm-restricted-networks)
- [Accessing images for Operators from private registries](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-accessing-images-private-registries_olm-managing-custom-catalogs)
- [Adding a catalog source to a cluster](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-creating-catalog-from-index_olm-restricted-networks)
- [ACM Deploy](https://github.com/stolostron/deploy)
- [Install in disconnected network environments](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.7/html/install/installing#install-on-disconnected-networks)
- [Mirroring images for a disconnected installation](https://docs.openshift.com/container-platform/4.11/installing/disconnected_install/installing-mirroring-installation-images.html#installing-mirroring-installation-images)
- [Operator SDK Integration with Operator Lifecycle Manager](https://sdk.operatorframework.io/docs/olm-integration/)
- [ManagedClusterImageRegistry CRD](https://github.com/stolostron/multicloud-operators-foundation/blob/main/docs/imageregistry/imageregistry.md)
