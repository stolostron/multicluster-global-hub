

# Installing the Standalone Agent Using Helm Chart

Follow these steps to deploy the standalone agent.

### Step 1: Deploy the Standalone Agent


Run the standalone agent using the following command:

```bash
# Name of the standalone agent (default: event-exporter). Change as needed.
APP_NAME=event-exporter 
# Namespace where the agent will be deployed (default: open-cluster-management). Change as needed.
APP_NAMESPACE=open-cluster-management

helm install $APP_NAME ./doc/standalone-agent -n $APP_NAMESPACE
```

### Step 2: Configure the `transport-config` Secret

The `transport-config` secret contains credentials that allow the `event-exporter` to deliver events. The secret schema is as follows:

- Secret Structure: `transport-config`

```yaml
apiVersion: v1
data:
  kafka.yaml: Ym9v...
kind: Secret
metadata:
  name: transport-config
  namespace: $APP_NAME
type: Opaque
```

- Content of `kafka.yaml`

```bash
$ oc get secret transport-config -ojsonpath='{.data.kafka\.yaml}' | base64 -d
bootstrap.server: kafka-...       
topic.status: topic...           # topic for sending events
ca.crt: LS0...
client.crt: LS0...
client.key: LS0...
```

### Step 3: Generate the `transport-config` Secret

You can either create the secret manually or generate it using the built-in Kafka instance in the global hub cluster. To generate the secret, set the following environment variables:

- `KUBECONFIG`: The kubeconfig file for the global hub cluster where the Strimzi Kafka instance is installed.
- `SECRET_KUBECONFIG`: The kubeconfig file for generating the `transport-config` secret
- `KAFKA_NAMESPACE`: The namespace in the global hub where Kafka is installed.
- `SECRET_NAMESPACE`: The namespace where the transport secret will be created.

For example, to generate the secret in the `open-cluster-management` namespace on the global hub cluster, run the following commands:

```bash
# kubeconfig, kafka instance is running in the current cluster: KUBECONFIG
export SECRET_KUBECONFIG=$KUBECONFIG

# namesapce
export KAFKA_NAMESPACE="multicluster-global-hub"
export SECRET_NAMESPACE="open-cluster-management"

./test/script/standalone_agent_secret.sh $KUBECONFIG $SECRET_KUBECONFIG
```

### Uninstallation

```bash
helm uninstall $APP_NAME -n $APP_NAMESPACE
```
