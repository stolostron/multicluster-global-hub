# Deploy Global Hub Operator on a Disconnected Environment 

## Prerequisites
- Make sure you have an image registry, and a workstation that has access to both the Internet and your mirror registry
- Have OLM installed on your cluster
- The ACM hub has been installed on your cluster
- Make sure your user is authorized with cluster-admin permissions
- The `operator-sdk` must be v1.22.0 or later

## Mirror Registry

Installing global hub in a disconnected environment involves the use of a mirror image registry. Which ensures your clusters only use container images that satisfy your organizational controls on external content. You can following the following two step to provision the mirror registry for global hub.
- [Creating a mirror registry](https://docs.openshift.com/container-platform/4.11/installing/disconnected_install/installing-mirroring-creating-registry.html#installing-mirroring-creating-registry)
- [Mirroring images for a disconnected installation](https://docs.openshift.com/container-platform/4.11/installing/disconnected_install/installing-mirroring-installation-images.html)

## Adding Global Hub Operator Catalog Source to a Cluster[Optional]
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

### Create the CatalogSource Object with the Pull Secret
```bash
$ cat ./doc/disconneted-operator/catalogsourc.yaml
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
  image: ${REGISTRY}/multicluster-global-hub-operator-catalog:v0.0.1
  publisher: global-hub-squad  

$ envsubst < ./doc/disconneted-operator/catalogsource.yaml | kubectl apply -f -
```
OLM polls catalog sources for available packages on a regular timed interval. After OLM polls the catalog source for your mirrored catalog, you can verify that the required packages are available from on your disconnected cluster by querying the available PackageManifest resources.
```bash
$ oc get packagemanifest -n openshift-marketplace multicluster-global-hub-operator
NAME                               CATALOG               AGE
multicluster-global-hub-operator   Community Operators   28m
```

## S
### Configure Image Content Source Policies
In order to have your cluster obtain container images for the global hub operator from your mirror registry, rather than from the internet-hosted registries, you must configure an ImageContentSourcePolicy on your disconnected cluster to redirect image references to your mirror registry.
```bash
$ cat ./doc/disconneted-operator/imagecontentpolicy.yaml 
apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: global-hub-operator-icsp
spec:
  repositoryDigestMirrors:
  - mirrors:
    - ${REGISTRY}/multicluster-global-hub-operator
    source: quay.io/stolostron/multicluster-global-hub-operator

$ envsubst < ./doc/disconneted-operator/imagecontentpolicy.yaml | kubectl apply -f -
```
If the Operator or Operand images that are referenced by a subscribed Operator require access to a private registry, you can either [provide access to all namespaces in the cluster, or individual target tenant namespaces](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-creating-catalog-from-index_olm-managing-custom-catalogs).

```bash
export USER=<the-registry-user>
export PASSWORD=<the-registry-password>
oc get secret/pull-secret -n openshift-config --template='{{index .data ".dockerconfigjson" | base64decode}}' >pull_secret.yaml
oc registry login --registry=${REGISTRY} --auth-basic="$USER:$PASSWORD" --to=pull_secret.yaml
oc set data secret/pull-secret -n openshift-config --from-file=.dockerconfigjson=pull_secret.yaml
rm pull_secret.yaml
```

### Installing Operator from the CLI
- Create the `OperatorGroup`
  
  Each namespace can have only one operator group. Replace `default` with the name of your operator group. Replace namespace with the name of your project namespace.
  ```bash
  $ cat ./doc/disconneted-operator/operatorgroup.yaml     
  apiVersion: operators.coreos.com/v1
  kind: OperatorGroup
  metadata:
    name: global-hub-operator-sdk-og
    namespace: default
  spec:
    targetNamespaces:
    - default
  $ oc apply -f ./doc/disconneted-operator/operatorgroup.yaml  
  ```
- Create the `Subscription`
  ```bash
  $ cat ./doc/disconneted-operator/subscription.yaml                                
  apiVersion: operators.coreos.com/v1alpha1
  kind: Subscription
  metadata:
    name: multicluster-global-hub-operator
    namespace: default
  spec:
    channel: prerelease-2.8
    installPlanApproval: Automatic
    name: multicluster-global-hub-operator
    source: global-hub-operator-catalog
    sourceNamespace: openshift-marketplace
  $ oc apply -f ./doc/disconneted-operator/subscription.yaml 
  ```
- Check the global hub operator
  ```bash
  $ oc get pods -n default
  NAME                                                READY   STATUS    RESTARTS   AGE
  multicluster-global-hub-operator-687584cb7c-fnftj   1/1     Running   0          12m
  ```

## Steps

### Build the Multicluster Global Hub Operator Image [Optional]
If you already have the global operator image in your registry, you can skip this step.
```bash
export REGISTRY=<operator-image-registry>
make build-operator-image && make push-operator-image
```

### Update the Bundle for Global Hub Operator
- If you specify a private registry, you need to append the `imagePullSecrets` in file `./operator/config/manager/manager.yaml`
```yaml
...
            cpu: 10m
            memory: 64Mi
      imagePullSecrets:
        - name: <image-pull-secret>
      serviceAccountName: multicluster-global-hub-operator
      terminationGracePeriodSeconds: 10
...
```
- Update the bundle with Global Hub Operator Image
```
export IMG=${REGISTRY}/multicluster-global-hub-operator:latest
cd ./operator
make bundle
cd ..
```

### Build the Bundle Image 
```bash
export VERSION=0.0.1
export IMAGE_TAG_BASE=${REGISTRY}/multicluster-global-hub-operator
cd ./operator
make bundle-build bundle-push catalog-build catalog-push
cd ..
```

### Running the Global Hub Operator with the Bundle Image
```bash
operator-sdk run bundle ${REGISTRY}/multicluster-global-hub-operator-bundle:v0.0.1 \
  --pull-secret-name ecr-image-pull-secret \
  --namespace=<provisioned namespace>
```

### Cleanup the Operator
```bash
operator-sdk cleanup multicluster-global-hub-operator --delete-all
```

## References
- [Mirroring an Operator catalog](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-mirror-catalog_olm-restricted-networks)
- [Accessing images for Operators from private registries](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-accessing-images-private-registries_olm-managing-custom-catalogs)
- [Adding a catalog source to a cluster](https://access.redhat.com/documentation/en-us/openshift_container_platform/4.11/html-single/operators/index#olm-creating-catalog-from-index_olm-restricted-networks)
- [ACM Deploy](https://github.com/stolostron/deploy)
- [Operator SDK Integration with Operator Lifecycle Manager](https://sdk.operatorframework.io/docs/olm-integration/)
- [Mirroring images for a disconnected installation](https://docs.openshift.com/container-platform/4.11/installing/disconnected_install/installing-mirroring-installation-images.html#installing-mirroring-installation-images)