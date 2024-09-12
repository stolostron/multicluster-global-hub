

# Installing the Event Exporter Using Helm Chart

Follow these steps to deploy the event exporter.

### Step 1: Deploy the Event Exporter


Run the event exporter using the following command:

```bash
# Name of the standalone agent (default: event-exporter). Change as needed.
APP_NAME=event-exporter 
# Namespace where the agent will be deployed (default: open-cluster-management). Change as needed.
APP_NAMESPACE=open-cluster-management

helm install $APP_NAME ./doc/event-exporter -n $APP_NAMESPACE
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

### Uninstallation

```bash
helm uninstall $APP_NAME -n $APP_NAMESPACE
```
