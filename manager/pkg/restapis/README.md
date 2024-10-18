# Multicluster Global Hub API

This documentation is for the APIs of multicluster global hub resources. Multicluster global hub contains three resource categories: managed clusters, policies, application subscriptions. Each type of resource has two possible requests: list, get.

## Get Started

1. Install multicluster global hub and create some global hub resources(manged clusters, policies, application subscriptions)
2. Log into the multicluster global hub cluster and retrieve the access token by running

```bash
export TOKEN=$(oc whoami -t)
```

3. Get the host of multicluster global hub API

```bash
export GLOBAL_HUB_API_HOST=$(oc -n multicluster-global-hub get route multicluster-global-hub-manager -o jsonpath={.spec.host})
```

4. Access the multicluster global hub API

- List managed clusters:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/managedclusters"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/managedclusters?limit=2"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/managedclusters?labelSelector=env%3Dproduction"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/managedclusters?labelSelector=env%3Dproduction&limit=2"
```

- Patch label for managed cluster:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" -X PATCH "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/managedcluster/<managed_cluster_uid>" -d '[{"op":"add","path":"/metadata/labels/foo","value":"bar"}]'
```

- List policies:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/policies"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/policies?limit=2"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/policies?labelSelector=env%3Dproduction"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/policies?labelSelector=env%3Dproduction&limit=2"
```

- Get policy status with policy ID:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/policy/<policy_uid>/status"
```

- List subscriptions:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/subscriptions"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/subscriptions?limit=2"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/subscriptions?labelSelector=env%3Dproduction"
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/subscriptions?labelSelector=env%3Dproduction&limit=2"
```

- Get subscription report with subscription ID:

```bash
curl -sk -H "Authorization: Bearer $TOKEN" "https://$GLOBAL_HUB_API_HOST/global-hub-api/v1/subscriptionreport/<sub_uid>"
```

## Contributing

If you want change the APIs, you need to follow the below steps to generate swagger document.

1. Generate the rough `swagger.yaml` or `swagger.json` with [swag](https://github.com/swaggo/swag)

```bash
swag init -g ./non_k8s_api.go -o . --outputTypes "yaml" --parseDependency
```

2. Edit the `swagger.yaml` or `swagger.json` manually for detailed API specification.

3. Generate swagger markdown document with [go-swagger](https://github.com/go-swagger/go-swagger)

```bash
swagger generate markdown -f ./swagger.yaml --output ./swagger.md
```
