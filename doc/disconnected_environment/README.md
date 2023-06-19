# Deploy Global Hub Operator on a Disconnected Environment 

## Prerequisites
- Make sure you have an image registry, and a bastion host that has access to both the Internet and your mirror registry
- Have OLM([Operator Lifecycle Manager](https://docs.openshift.com/container-platform/4.11/operators/understanding/olm/olm-understanding-olm.html)) installed on your cluster
- The Advanced Cluster Management for Kubernetes has been installed on your cluster
- Make sure your user is authorized with cluster-admin permissions

## Mirror Registry

Installing global hub in a disconnected environment involves the use of a mirror image registry. Which ensures your clusters only use container images that satisfy your organizational controls on external content. You can following the following two step to provision the mirror registry for global hub.
- [Creating a mirror registry](https://docs.openshift.com/container-platform/4.11/installing/disconnected_install/installing-mirroring-creating-registry.html#installing-mirroring-creating-registry)
- [Mirroring images for a disconnected installation](https://docs.openshift.com/container-platform/4.11/installing/disconnected_install/installing-mirroring-installation-images.html)

## Add Global Hub Operator CatalogSource to a Cluster [Optional]
### Build the Global Hub Bundle Image and Index Image
```bash
export REGISTRY=<operator-mirror-registry>
export VERSION=0.0.1
export IMAGE_TAG_BASE=${REGISTRY}/multicluster-global-hub-operator

cd ./operator
# update bundle
make generate manifests bundle
# build bundle image
make bundle-build bundle-push catalog-build catalog-push
cd ..
```
After running the above command, the following images have been built and pushed to the `$REGISTRY`.
- Bundle Image: `${REGISTRY}/multicluster-global-hub-operator-bundle:v0.0.1`
- Catalog Image: `${REGISTRY}/multicluster-global-hub-operator-catalog:v0.0.1`

### Prepare Image Pull Secret for the Catalog
- Create a secret for each required private registry.
  - Login the registry `docker login $REGISTRY`
  - File storing credentials for the registry
    ```json
    {
        "auths": {
                "$REGISTRY": {
                        "auth": "Xd2lhdsbnRib21iMQ=="
                }
        }
    }
    ```

  - Create the secret from the credentials
    ```bash
    oc create secret generic global-hub-secret \
      -n openshift-marketplace \
      --from-file=.dockerconfigjson=/root/.docker/config.json \
      --type=kubernetes.io/dockerconfigjson
    ```

### Create the CatalogSource Object with the Image Pull Secret
```bash
$ cat ./doc/disconnected-operator/catalogsource.yaml
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
  - global-hub-secret
  image: ${REGISTRY}/multicluster-global-hub-operator-catalog:v${VERSION}
  publisher: global-hub-squad  

$ envsubst < ./doc/disconnected-operator/catalogsource.yaml | kubectl apply -f -
```
OLM polls catalog sources for available packages on a regular timed interval. After OLM polls the catalog source for your mirrored catalog, you can verify that the required packages are available from on your disconnected cluster by querying the available PackageManifest resources.
```bash
$ oc get packagemanifest -n openshift-marketplace multicluster-global-hub-operator
NAME                               CATALOG               AGE
multicluster-global-hub-operator   Community Operators   28m
```

## Configure the Image Pull Secret
### Create Image Content Source Policies
In order to have your cluster obtain container images for the global hub operator from your mirror registry, rather than from the internet-hosted registries, you must configure an `ImageContentSourcePolicy` on your disconnected cluster to redirect image references to your mirror registry.(Note: You need to use the operator image with digest, otherwise the ImageContentSourcePolicy configuration will not take effect. export the `IMG` variable to update operator image in the CSV when build the bundle image for testing)
```bash
cat ./doc/disconnected-operator/imagecontentpolicy.yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: global-hub-operator-icsp
spec:
  repositoryDigestMirrors:
  - mirrors:
    - ${REGISTRY}/multicluster-global-hub-operator
    source: quay.io/stolostron/multicluster-global-hub-operator

$ envsubst < ./doc/disconnected-operator/imagecontentpolicy.yaml | kubectl apply -f -
```

### Create Image Pull Secret for the Operator or Operand

If the Operator or Operand images that are referenced by a subscribed Operator require access to a private registry, you can either [provide access to all namespaces in the cluster, or individual target tenant namespaces](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-creating-catalog-from-index_olm-managing-custom-catalogs). 

#### Sample 1. Configure image pull secret to all the namespaces on a Openshift cluster
Caution: if you apply this on a pre-existing cluster, it will cause a rolling restart of all nodes.
```bash
export USER=<the-registry-user>
export PASSWORD=<the-registry-password>
oc get secret/pull-secret -n openshift-config --template='{{index .data ".dockerconfigjson" | base64decode}}' >pull_secret.yaml
oc registry login --registry=${REGISTRY} --auth-basic="$USER:$PASSWORD" --to=pull_secret.yaml
oc set data secret/pull-secret -n openshift-config --from-file=.dockerconfigjson=pull_secret.yaml
rm pull_secret.yaml
```

#### Sample 2. Configure image pull secret to an individual namespace
```bash
# create the secret in the tenant namespace
oc create secret generic <secret_name> \
    -n <tenant_namespace> \
    --from-file=.dockerconfigjson=<path/to/registry/credentials> \
    --type=kubernetes.io/dockerconfigjson

# link the secret to the service account for your operator/operand
oc secrets link <operator_sa> -n <tenant_namespace> <secret_name> --for=pull
```

## Install the Global Hub Operator
### Install the Operator from OperatorHub using the CLI
- Create the `OperatorGroup`
  
  Each namespace can have only one operator group. Replace `default` with the name of your operator group. Replace namespace with the name of your project namespace.
  ```bash
  $ cat ./doc/disconnected-operator/operatorgroup.yaml  
  apiVersion: operators.coreos.com/v1
  kind: OperatorGroup
  metadata:
    name: global-hub-operator-sdk-og
    namespace: default
  spec:
    targetNamespaces:
    - default

  $ oc apply -f ./doc/disconnected-operator/operatorgroup.yaml   
  ```
 
- Create the `Subscription`
  ```bash
  $ cat ./doc/disconnected-operator/subscription.yaml
  apiVersion: operators.coreos.com/v1alpha1
  kind: Subscription
  metadata:
    name: multicluster-global-hub-operator
    namespace: default
  spec:
    channel: alpha
    installPlanApproval: Automatic
    name: multicluster-global-hub-operator
    source: global-hub-operator-catalog
    sourceNamespace: openshift-marketplace

  $ oc apply -f ./doc/disconnected-operator/subscription.yaml
  ```
  
- Check the global hub operator
  ```bash
  $ oc get pods -n default
  NAME                                                READY   STATUS    RESTARTS   AGE
  multicluster-global-hub-operator-687584cb7c-fnftj   1/1     Running   0          2m12s
  $ oc describe pod multicluster-global-hub-operator-687584cb7c-fnftj
  ...
  Events:
  Type    Reason          Age    From               Message
  ----    ------          ----   ----               -------
  Normal  Scheduled       2m52s  default-scheduler  Successfully assigned default/multicluster-global-hub-operator-5546668786-f7b7v to ip-10-0-137-91.ec2.internal
  Normal  AddedInterface  2m50s  multus             Add eth0 [10.128.1.7/23] from openshift-sdn
  Normal  Pulling         2m49s  kubelet            Pulling image "quay.io/stolostron/multicluster-global-hub-operator@sha256:f385a9cfa78442526d6721fc7aa182ec6b98dffdabc78e2732bf9adbc5c8e0df"
  Normal  Pulled          2m35s  kubelet            Successfully pulled image "quay.io/stolostron/multicluster-global-hub-operator@sha256:f385a9cfa78442526d6721fc7aa182ec6b98dffdabc78e2732bf9adbc5c8e0df" in 14.180033246s
  Normal  Created         2m35s  kubelet            Created container multicluster-global-hub-operator
  Normal  Started         2m35s  kubelet            Started container multicluster-global-hub-operator
  ...
  ```
  In a disconnection environment, although it can be seen from the events in the operator pod that the image is pulled from the public registry `quay.io/stolostron`, it is actually pulled from the `$REGISTRY` configured by the `ImageContentSourcePolicy`

### Install the Operator from OperatorHub using the web console
You can install and subscribe to an Operator from OperatorHub using the OpenShift Container Platform web console. For more details, please refer [here](https://docs.openshift.com/container-platform/4.11/operators/admin/olm-adding-operators-to-cluster.html)


## Image Registries Configuration

The following types of image should be consider when running the global hub.

- Index image: `${REGISTRY}/multicluster-global-hub-operator-catalog`
- Bundle image: `${REGISTRY}/multicluster-global-hub-operator-bundle`
- Operator image: `quay.io/stolostron/multicluster-global-hub-operator`
- Operand Images
  - Manager image: `quay.io/stolostron/multicluster-global-hub-manager`
  - Grafana image: `quay.io/stolostron/grafana`
  - Oauth Proxy image: `quay.io/stolostron/origin-oauth-proxy`
  - Agent image: `quay.io/stolostron/multicluster-global-hub-agent`

In most cases the customers don't need to pay attention to the first three images, because they are used to build the global hub CatalogSource. So we will mainly describe how to configure the registries and pull secret of operand images

### Configure the Operand Image Registry by Annotation

This is achieved by adding annotation to the MGH CR and specifying the image pull secret and image pull policy. e.g:
```yaml
apiVersion: operator.open-cluster-management.io/v1alpha3
kind: MulticlusterGlobalHub
metadata:
  annotations:
    mgh-image-repository: <private-image-registry>
  name: multiclusterglobalhub
  namespace: open-cluster-management
spec:
  imagePullPolicy: Always
  imagePullSecret: ecr-image-pull-secret
  dataLayer:
    type: largeScale
```
The advantage of this way is that it's easy to configure. But because the **Agent image** runs in the regional hub cluster. And different clusters might have different registries, then the above method cannot reach this situation. Here is another way to achieve this.

### Config the Agent Image Registry By ManagedClusterImageRegistry

- Create the placement/clusterset to select the target regional hub cluster, for example:
  ```yaml
  apiVersion: cluster.open-cluster-management.io/v1beta1
  kind: Placement
  metadata:
    name: <placement-name>
    namespace: <placement-namespace>
  spec:
    clusterSets:
      - <cluster-set>
    predicates:
    - requiredClusterSelector:
        labelSelector:
          matchLabels:
            <key>: <label>
  ---
  apiVersion: cluster.open-cluster-management.io/v1beta1
  kind: ManagedClusterSetBinding
  metadata:
    name: <cluster-set>
    namespace: <placement-namespace>
  spec:
    clusterSet: <cluster-set>
  ---
  apiVersion: cluster.open-cluster-management.io/v1beta1
  kind: ManagedClusterSet
  metadata:
    name: <cluster-set>
  spec:
    clusterSelector:
      selectorType: LegacyClusterSetLabel
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
        source: <image-will-be-overridden-by-mirror>
  ```
By configuring the above, a label and an annotation will be added to the selected `ManagedCluster`. This means that the agent image in the cluster will be replaced with the mirror image.
  - Label: `open-cluster-management.io/image-registry=Namespace.ManagedClusterImageRegistryName`
  - Annotation: `open-cluster-management.io/image-registries`


## References
- [Mirroring an Operator catalog](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-mirror-catalog_olm-restricted-networks)
- [Accessing images for Operators from private registries](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-accessing-images-private-registries_olm-managing-custom-catalogs)
- [Adding a catalog source to a cluster](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-creating-catalog-from-index_olm-restricted-networks)
- [ACM Deploy](https://github.com/stolostron/deploy)
- [Install in disconnected network environments](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.7/html/install/installing#install-on-disconnected-networks)
- [Mirroring images for a disconnected installation](https://docs.openshift.com/container-platform/4.11/installing/disconnected_install/installing-mirroring-installation-images.html#installing-mirroring-installation-images)
- [Operator SDK Integration with Operator Lifecycle Manager](https://sdk.operatorframework.io/docs/olm-integration/)
- [ManagedClusterImageRegistry CRD](https://github.com/stolostron/multicloud-operators-foundation/blob/main/docs/imageregistry/imageregistry.md)
