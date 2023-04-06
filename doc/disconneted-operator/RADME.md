# Deploy Global Hub Operator on a Disconnected Environment 

## Prerequisites
- Make sure you have an image registry
- The corresponding `imagePullSecret` has been created on your cluster
- Make sure your user is authorized with cluster-admin permissions
- Have OLM installed on your cluster
- The `operator-sdk` must be v1.22.0 or later

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
make bundle-build bundle-push
cd ..
```

### Running the Global Hub Operator with the Bundle Image
```bash
operator-sdk run bundle ${REGISTRY}/multicluster-global-hub-operator-bundle:v0.0.1 \
    --pull-secret-name <image-pull-secret>
```

### Cleanup the Operator
```bash
operator-sdk cleanup multicluster-global-hub-operator --delete-all
```

## References
- [Operator SDK Integration with Operator Lifecycle Manager](https://sdk.operatorframework.io/docs/olm-integration/)
- [Mirroring images for a disconnected installation](https://docs.openshift.com/container-platform/4.11/installing/disconnected_install/installing-mirroring-installation-images.html#installing-mirroring-installation-images)
