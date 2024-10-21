


# Multicluster Global Hub API
This documentation is for the APIs of multicluster global hub resources for {product-title}. Multicluster
global hub contains three resource categories: managed clusters, policies, application subscriptions.
Each type of resource has two possible requests: list, get.
  

## Informations

### Version

1.0.0

### License

[Apache 2.0](http://www.apache.org/licenses/LICENSE-2.0.html)

### Contact

acm-contact acm-contact@redhat.com https://github.com/stolostron/multicluster-global-hub

## Tags

  ### <span id="tag-cluster-open-cluster-management-io"></span>[cluster.open-cluster-management.io](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/apis/apis#clusters-api)

Access to clusters

  ### <span id="tag-policy-open-cluster-management-io"></span>[policy.open-cluster-management.io](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/apis/apis#policy-api)

Access to policies

  ### <span id="tag-apps-open-cluster-management-io"></span>[apps.open-cluster-management.io](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/apis/apis#subscriptions-api)

Access to application subscriptions

## Content negotiation

### URI Schemes
  * http
  * https

### Consumes
  * application/json

### Produces
  * application/json

## Access control

### Security Schemes

#### ApiKeyAuth (header: Authorization)



> **Type**: apikey

## All endpoints

###  apps_open_cluster_management_io

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /global-hub-api/v1/subscriptionreport/{subscriptionID} | [get subscriptionreport subscription ID](#get-subscriptionreport-subscription-id) | get application subscription report |
| GET | /global-hub-api/v1/subscriptions | [get subscriptions](#get-subscriptions) | list application subscriptions |
  


###  cluster_open_cluster_management_io

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /global-hub-api/v1/managedclusters | [get managedclusters](#get-managedclusters) | list managed clusters |
| PATCH | /global-hub-api/v1/managedcluster/{clusterID} | [patch managedcluster cluster ID](#patch-managedcluster-cluster-id) | patch managed cluster label |
  


###  policy_open_cluster_management_io

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /global-hub-api/v1/policies | [get policies](#get-policies) | list policies |
| GET | /global-hub-api/v1/policy/{policyID}/status | [get policy policy ID status](#get-policy-policy-id-status) | get policy status |
  


## Paths

### <span id="get-managedclusters"></span> list managed clusters (*GetManagedclusters*)

```
GET /global-hub-api/v1/managedclusters
```

list managed clusters

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * ApiKeyAuth

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| continue | `query` | string | `string` |  |  |  | Continue token to request next request. As an API client, you can then pass this continue value to the API server on the next request, to instruct the server to return the next page of results. By continuing until the server returns an empty continue value, you can retrieve the entire collection. |
| labelSelector | `query` | string | `string` |  |  |  | list managed clusters by label selector |
| limit | `query` | integer | `int64` |  |  |  | maximum managed cluster number to receive |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#get-managedclusters-200) | OK | OK |  | [schema](#get-managedclusters-200-schema) |
| [400](#get-managedclusters-400) | Bad Request | Bad Request |  | [schema](#get-managedclusters-400-schema) |
| [401](#get-managedclusters-401) | Unauthorized | Unauthorized |  | [schema](#get-managedclusters-401-schema) |
| [403](#get-managedclusters-403) | Forbidden | Forbidden |  | [schema](#get-managedclusters-403-schema) |
| [404](#get-managedclusters-404) | Not Found | Not Found |  | [schema](#get-managedclusters-404-schema) |
| [500](#get-managedclusters-500) | Internal Server Error | Internal Server Error |  | [schema](#get-managedclusters-500-schema) |
| [503](#get-managedclusters-503) | Service Unavailable | Service Unavailable |  | [schema](#get-managedclusters-503-schema) |

#### Responses


##### <span id="get-managedclusters-200"></span> 200 - OK
Status: OK

###### <span id="get-managedclusters-200-schema"></span> Schema
   
  

[ManagedClusterList](#managed-cluster-list)

##### <span id="get-managedclusters-400"></span> 400 - Bad Request
Status: Bad Request

###### <span id="get-managedclusters-400-schema"></span> Schema

##### <span id="get-managedclusters-401"></span> 401 - Unauthorized
Status: Unauthorized

###### <span id="get-managedclusters-401-schema"></span> Schema

##### <span id="get-managedclusters-403"></span> 403 - Forbidden
Status: Forbidden

###### <span id="get-managedclusters-403-schema"></span> Schema

##### <span id="get-managedclusters-404"></span> 404 - Not Found
Status: Not Found

###### <span id="get-managedclusters-404-schema"></span> Schema

##### <span id="get-managedclusters-500"></span> 500 - Internal Server Error
Status: Internal Server Error

###### <span id="get-managedclusters-500-schema"></span> Schema

##### <span id="get-managedclusters-503"></span> 503 - Service Unavailable
Status: Service Unavailable

###### <span id="get-managedclusters-503-schema"></span> Schema

### <span id="get-policies"></span> list policies (*GetPolicies*)

```
GET /global-hub-api/v1/policies
```

list policies

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * ApiKeyAuth

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| continue | `query` | string | `string` |  |  |  | Continue token to request next request. As an API client, you can then pass this continue value to the API server on the next request, to instruct the server to return the next page of results. By continuing until the server returns an empty continue value, you can retrieve the entire collection. |
| labelSelector | `query` | string | `string` |  |  |  | list policies by label selector |
| limit | `query` | integer | `int64` |  |  |  | maximum policy number to receive |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#get-policies-200) | OK | OK |  | [schema](#get-policies-200-schema) |
| [400](#get-policies-400) | Bad Request | Bad Request |  | [schema](#get-policies-400-schema) |
| [401](#get-policies-401) | Unauthorized | Unauthorized |  | [schema](#get-policies-401-schema) |
| [403](#get-policies-403) | Forbidden | Forbidden |  | [schema](#get-policies-403-schema) |
| [404](#get-policies-404) | Not Found | Not Found |  | [schema](#get-policies-404-schema) |
| [500](#get-policies-500) | Internal Server Error | Internal Server Error |  | [schema](#get-policies-500-schema) |
| [503](#get-policies-503) | Service Unavailable | Service Unavailable |  | [schema](#get-policies-503-schema) |

#### Responses


##### <span id="get-policies-200"></span> 200 - OK
Status: OK

###### <span id="get-policies-200-schema"></span> Schema
   
  

[PolicyList](#policy-list)

##### <span id="get-policies-400"></span> 400 - Bad Request
Status: Bad Request

###### <span id="get-policies-400-schema"></span> Schema

##### <span id="get-policies-401"></span> 401 - Unauthorized
Status: Unauthorized

###### <span id="get-policies-401-schema"></span> Schema

##### <span id="get-policies-403"></span> 403 - Forbidden
Status: Forbidden

###### <span id="get-policies-403-schema"></span> Schema

##### <span id="get-policies-404"></span> 404 - Not Found
Status: Not Found

###### <span id="get-policies-404-schema"></span> Schema

##### <span id="get-policies-500"></span> 500 - Internal Server Error
Status: Internal Server Error

###### <span id="get-policies-500-schema"></span> Schema

##### <span id="get-policies-503"></span> 503 - Service Unavailable
Status: Service Unavailable

###### <span id="get-policies-503-schema"></span> Schema

### <span id="get-policy-policy-id-status"></span> get policy status (*GetPolicyPolicyIDStatus*)

```
GET /global-hub-api/v1/policy/{policyID}/status
```

get status with a given policy

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * ApiKeyAuth

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| policyID | `path` | string | `string` |  | ✓ |  | Policy ID |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#get-policy-policy-id-status-200) | OK | OK |  | [schema](#get-policy-policy-id-status-200-schema) |
| [400](#get-policy-policy-id-status-400) | Bad Request | Bad Request |  | [schema](#get-policy-policy-id-status-400-schema) |
| [401](#get-policy-policy-id-status-401) | Unauthorized | Unauthorized |  | [schema](#get-policy-policy-id-status-401-schema) |
| [403](#get-policy-policy-id-status-403) | Forbidden | Forbidden |  | [schema](#get-policy-policy-id-status-403-schema) |
| [404](#get-policy-policy-id-status-404) | Not Found | Not Found |  | [schema](#get-policy-policy-id-status-404-schema) |
| [500](#get-policy-policy-id-status-500) | Internal Server Error | Internal Server Error |  | [schema](#get-policy-policy-id-status-500-schema) |
| [503](#get-policy-policy-id-status-503) | Service Unavailable | Service Unavailable |  | [schema](#get-policy-policy-id-status-503-schema) |

#### Responses


##### <span id="get-policy-policy-id-status-200"></span> 200 - OK
Status: OK

###### <span id="get-policy-policy-id-status-200-schema"></span> Schema
   
  

[][Policy](#policy)

##### <span id="get-policy-policy-id-status-400"></span> 400 - Bad Request
Status: Bad Request

###### <span id="get-policy-policy-id-status-400-schema"></span> Schema

##### <span id="get-policy-policy-id-status-401"></span> 401 - Unauthorized
Status: Unauthorized

###### <span id="get-policy-policy-id-status-401-schema"></span> Schema

##### <span id="get-policy-policy-id-status-403"></span> 403 - Forbidden
Status: Forbidden

###### <span id="get-policy-policy-id-status-403-schema"></span> Schema

##### <span id="get-policy-policy-id-status-404"></span> 404 - Not Found
Status: Not Found

###### <span id="get-policy-policy-id-status-404-schema"></span> Schema

##### <span id="get-policy-policy-id-status-500"></span> 500 - Internal Server Error
Status: Internal Server Error

###### <span id="get-policy-policy-id-status-500-schema"></span> Schema

##### <span id="get-policy-policy-id-status-503"></span> 503 - Service Unavailable
Status: Service Unavailable

###### <span id="get-policy-policy-id-status-503-schema"></span> Schema

### <span id="get-subscriptionreport-subscription-id"></span> get application subscription report (*GetSubscriptionreportSubscriptionID*)

```
GET /global-hub-api/v1/subscriptionreport/{subscriptionID}
```

get report for a given application subscription

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * ApiKeyAuth

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| subscriptionID | `path` | string | `string` |  | ✓ |  | Subscription ID |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#get-subscriptionreport-subscription-id-200) | OK | OK |  | [schema](#get-subscriptionreport-subscription-id-200-schema) |
| [400](#get-subscriptionreport-subscription-id-400) | Bad Request | Bad Request |  | [schema](#get-subscriptionreport-subscription-id-400-schema) |
| [401](#get-subscriptionreport-subscription-id-401) | Unauthorized | Unauthorized |  | [schema](#get-subscriptionreport-subscription-id-401-schema) |
| [403](#get-subscriptionreport-subscription-id-403) | Forbidden | Forbidden |  | [schema](#get-subscriptionreport-subscription-id-403-schema) |
| [404](#get-subscriptionreport-subscription-id-404) | Not Found | Not Found |  | [schema](#get-subscriptionreport-subscription-id-404-schema) |
| [500](#get-subscriptionreport-subscription-id-500) | Internal Server Error | Internal Server Error |  | [schema](#get-subscriptionreport-subscription-id-500-schema) |
| [503](#get-subscriptionreport-subscription-id-503) | Service Unavailable | Service Unavailable |  | [schema](#get-subscriptionreport-subscription-id-503-schema) |

#### Responses


##### <span id="get-subscriptionreport-subscription-id-200"></span> 200 - OK
Status: OK

###### <span id="get-subscriptionreport-subscription-id-200-schema"></span> Schema
   
  

[SubscriptionReport](#subscription-report)

##### <span id="get-subscriptionreport-subscription-id-400"></span> 400 - Bad Request
Status: Bad Request

###### <span id="get-subscriptionreport-subscription-id-400-schema"></span> Schema

##### <span id="get-subscriptionreport-subscription-id-401"></span> 401 - Unauthorized
Status: Unauthorized

###### <span id="get-subscriptionreport-subscription-id-401-schema"></span> Schema

##### <span id="get-subscriptionreport-subscription-id-403"></span> 403 - Forbidden
Status: Forbidden

###### <span id="get-subscriptionreport-subscription-id-403-schema"></span> Schema

##### <span id="get-subscriptionreport-subscription-id-404"></span> 404 - Not Found
Status: Not Found

###### <span id="get-subscriptionreport-subscription-id-404-schema"></span> Schema

##### <span id="get-subscriptionreport-subscription-id-500"></span> 500 - Internal Server Error
Status: Internal Server Error

###### <span id="get-subscriptionreport-subscription-id-500-schema"></span> Schema

##### <span id="get-subscriptionreport-subscription-id-503"></span> 503 - Service Unavailable
Status: Service Unavailable

###### <span id="get-subscriptionreport-subscription-id-503-schema"></span> Schema

### <span id="get-subscriptions"></span> list application subscriptions (*GetSubscriptions*)

```
GET /global-hub-api/v1/subscriptions
```

list application subscriptions

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * ApiKeyAuth

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| continue | `query` | string | `string` |  |  |  | Continue token to request next request. As an API client, you can then pass this continue value to the API server on the next request, to instruct the server to return the next page of results. By continuing until the server returns an empty continue value, you can retrieve the entire collection. |
| labelSelector | `query` | string | `string` |  |  |  | list application subscriptions by label selector |
| limit | `query` | integer | `int64` |  |  |  | maximum application subscription number to receive |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#get-subscriptions-200) | OK | OK |  | [schema](#get-subscriptions-200-schema) |
| [400](#get-subscriptions-400) | Bad Request | Bad Request |  | [schema](#get-subscriptions-400-schema) |
| [401](#get-subscriptions-401) | Unauthorized | Unauthorized |  | [schema](#get-subscriptions-401-schema) |
| [403](#get-subscriptions-403) | Forbidden | Forbidden |  | [schema](#get-subscriptions-403-schema) |
| [404](#get-subscriptions-404) | Not Found | Not Found |  | [schema](#get-subscriptions-404-schema) |
| [500](#get-subscriptions-500) | Internal Server Error | Internal Server Error |  | [schema](#get-subscriptions-500-schema) |
| [503](#get-subscriptions-503) | Service Unavailable | Service Unavailable |  | [schema](#get-subscriptions-503-schema) |

#### Responses


##### <span id="get-subscriptions-200"></span> 200 - OK
Status: OK

###### <span id="get-subscriptions-200-schema"></span> Schema
   
  

[SubscriptionList](#subscription-list)

##### <span id="get-subscriptions-400"></span> 400 - Bad Request
Status: Bad Request

###### <span id="get-subscriptions-400-schema"></span> Schema

##### <span id="get-subscriptions-401"></span> 401 - Unauthorized
Status: Unauthorized

###### <span id="get-subscriptions-401-schema"></span> Schema

##### <span id="get-subscriptions-403"></span> 403 - Forbidden
Status: Forbidden

###### <span id="get-subscriptions-403-schema"></span> Schema

##### <span id="get-subscriptions-404"></span> 404 - Not Found
Status: Not Found

###### <span id="get-subscriptions-404-schema"></span> Schema

##### <span id="get-subscriptions-500"></span> 500 - Internal Server Error
Status: Internal Server Error

###### <span id="get-subscriptions-500-schema"></span> Schema

##### <span id="get-subscriptions-503"></span> 503 - Service Unavailable
Status: Service Unavailable

###### <span id="get-subscriptions-503-schema"></span> Schema

### <span id="patch-managedcluster-cluster-id"></span> patch managed cluster label (*PatchManagedclusterClusterID*)

```
PATCH /global-hub-api/v1/managedcluster/{clusterID}
```

patch label for a given managed cluster

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * ApiKeyAuth

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| clusterID | `path` | string | `string` |  | ✓ |  | Managed Cluster ID |
| patch | `body` | [ManagedClusterLabelPatch](#managed-cluster-label-patch) | `models.ManagedClusterLabelPatch` | | ✓ | | JSON patch that operators on managed cluster label |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#patch-managedcluster-cluster-id-200) | OK | OK |  | [schema](#patch-managedcluster-cluster-id-200-schema) |
| [400](#patch-managedcluster-cluster-id-400) | Bad Request | Bad Request |  | [schema](#patch-managedcluster-cluster-id-400-schema) |
| [401](#patch-managedcluster-cluster-id-401) | Unauthorized | Unauthorized |  | [schema](#patch-managedcluster-cluster-id-401-schema) |
| [403](#patch-managedcluster-cluster-id-403) | Forbidden | Forbidden |  | [schema](#patch-managedcluster-cluster-id-403-schema) |
| [404](#patch-managedcluster-cluster-id-404) | Not Found | Not Found |  | [schema](#patch-managedcluster-cluster-id-404-schema) |
| [500](#patch-managedcluster-cluster-id-500) | Internal Server Error | Internal Server Error |  | [schema](#patch-managedcluster-cluster-id-500-schema) |
| [503](#patch-managedcluster-cluster-id-503) | Service Unavailable | Service Unavailable |  | [schema](#patch-managedcluster-cluster-id-503-schema) |

#### Responses


##### <span id="patch-managedcluster-cluster-id-200"></span> 200 - OK
Status: OK

###### <span id="patch-managedcluster-cluster-id-200-schema"></span> Schema
   
  

[ManagedCluster](#managed-cluster)

##### <span id="patch-managedcluster-cluster-id-400"></span> 400 - Bad Request
Status: Bad Request

###### <span id="patch-managedcluster-cluster-id-400-schema"></span> Schema

##### <span id="patch-managedcluster-cluster-id-401"></span> 401 - Unauthorized
Status: Unauthorized

###### <span id="patch-managedcluster-cluster-id-401-schema"></span> Schema

##### <span id="patch-managedcluster-cluster-id-403"></span> 403 - Forbidden
Status: Forbidden

###### <span id="patch-managedcluster-cluster-id-403-schema"></span> Schema

##### <span id="patch-managedcluster-cluster-id-404"></span> 404 - Not Found
Status: Not Found

###### <span id="patch-managedcluster-cluster-id-404-schema"></span> Schema

##### <span id="patch-managedcluster-cluster-id-500"></span> 500 - Internal Server Error
Status: Internal Server Error

###### <span id="patch-managedcluster-cluster-id-500-schema"></span> Schema

##### <span id="patch-managedcluster-cluster-id-503"></span> 503 - Service Unavailable
Status: Service Unavailable

###### <span id="patch-managedcluster-cluster-id-503-schema"></span> Schema

## Models

### <span id="allow-deny-item"></span> AllowDenyItem


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| apiVersion | string| `string` |  | |  |  |
| kinds | []string| `[]string` |  | |  |  |



### <span id="ansible-jobs-status"></span> AnsibleJobsStatus


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| lastposthookjob | string| `string` |  | |  |  |
| lastprehookjob | string| `string` |  | |  |  |
| posthookjobshistory | []string| `[]string` |  | |  |  |
| prehookjobshistory | []string| `[]string` |  | |  |  |



### <span id="client-config"></span> ClientConfig


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| caBundle | []integer| `[]int64` |  | | CABundle is the ca bundle to connect to apiserver of the managed cluster.
System certs are used if it is not set.
+optional |  |
| url | string| `string` |  | | URL is the URL of apiserver endpoint of the managed cluster.
+required |  |



### <span id="cluster-override"></span> ClusterOverride


  

[interface{}](#interface)

### <span id="cluster-overrides"></span> ClusterOverrides


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| clusterName | string| `string` |  | |  |  |
| clusterOverrides | [][ClusterOverride](#cluster-override)| `[]ClusterOverride` |  | | +kubebuilder:validation:MinItems=1 |  |



### <span id="compliance-history"></span> ComplianceHistory


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| eventName | string| `string` |  | |  |  |
| lastTimestamp | string| `string` |  | |  |  |
| message | string| `string` |  | |  |  |



### <span id="compliance-per-cluster-status"></span> CompliancePerClusterStatus


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| clustername | string| `string` |  | |  |  |
| clusternamespace | string| `string` |  | |  |  |
| compliant | string| `string` |  | |  |  |



### <span id="condition"></span> Condition


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| lastTransitionTime | string| `string` |  | | lastTransitionTime is the last time the condition transitioned from one status to another.
This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.
+required
+kubebuilder:validation:Required
+kubebuilder:validation:Type=string
+kubebuilder:validation:Format=date-time |  |
| message | string| `string` |  | | message is a human readable message indicating details about the transition.
This may be an empty string.
+required
+kubebuilder:validation:Required
+kubebuilder:validation:MaxLength=32768 |  |
| observedGeneration | integer| `int64` |  | | observedGeneration represents the .metadata.generation that the condition was set based upon.
For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
with respect to the current state of the instance.
+optional
+kubebuilder:validation:Minimum=0 |  |
| reason | string| `string` |  | | reason contains a programmatic identifier indicating the reason for the condition's last transition.
Producers of specific condition types may define expected values and meanings for this field,
and whether the values are considered a guaranteed API.
The value should be a CamelCase string.
This field may not be empty.
+required
+kubebuilder:validation:Required
+kubebuilder:validation:MaxLength=1024
+kubebuilder:validation:MinLength=1
+kubebuilder:validation:Pattern=`^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$` |  |
| status | string| `string` |  | | status of the condition, one of True, False, Unknown.
+required
+kubebuilder:validation:Required
+kubebuilder:validation:Enum=True;False;Unknown |  |
| type | string| `string` |  | | type of condition in CamelCase or in foo.example.com/CamelCase.
---
Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
useful (see .node.status.conditions), the ability to deconflict is important.
The regex it matches is (dns1123SubdomainFmt/)?(qualifiedNameFmt)
+required
+kubebuilder:validation:Required
+kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
+kubebuilder:validation:MaxLength=316 |  |



### <span id="details-per-template"></span> DetailsPerTemplate


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| compliant | string| `string` |  | | +kubebuilder:validation:Enum=Compliant;NonCompliant |  |
| history | [][ComplianceHistory](#compliance-history)| `[]*ComplianceHistory` |  | |  |  |
| templateMeta | [ObjectMeta](#object-meta)| `ObjectMeta` |  | | +kubebuilder:pruning:PreserveUnknownFields |  |



### <span id="generic-cluster-reference"></span> GenericClusterReference


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| name | string| `string` |  | |  |  |



### <span id="hour-range"></span> HourRange


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| end | string| `string` |  | |  |  |
| start | string| `string` |  | |  |  |



### <span id="label-selector"></span> LabelSelector


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| matchExpressions | [][LabelSelectorRequirement](#label-selector-requirement)| `[]*LabelSelectorRequirement` |  | | matchExpressions is a list of label selector requirements. The requirements are ANDed.
+optional |  |
| matchLabels | map of string| `map[string]string` |  | | matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
map is equivalent to an element of matchExpressions, whose key field is "key", the
operator is "In", and the values array contains only "value". The requirements are ANDed.
+optional |  |



### <span id="label-selector-requirement"></span> LabelSelectorRequirement


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| key | string| `string` |  | | key is the label key that the selector applies to.
+patchMergeKey=key
+patchStrategy=merge |  |
| operator | string| `string` |  | | operator represents a key's relationship to a set of values.
Valid operators are In, NotIn, Exists and DoesNotExist. |  |
| values | []string| `[]string` |  | | values is an array of string values. If the operator is In or NotIn,
the values array must be non-empty. If the operator is Exists or DoesNotExist,
the values array must be empty. This array is replaced during a strategic
merge patch.
+optional |  |



### <span id="list-metadata"></span> ListMetadata


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| continue | string| `string` |  | | continue may be set if the user set a limit on the number of items returned, and indicates that
the server has more data available. The value is opaque and may be used to issue another request
to the endpoint that served this list to retrieve the next set of available objects. Continuing a
consistent list may not be possible if the server configuration has changed or more than a few
minutes have passed. The resourceVersion field returned when using this continue value will be
identical to the value in the first response, unless you have received this token from an error
message. |  |



### <span id="local-object-reference"></span> LocalObjectReference


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| name | string| `string` |  | | Name of the referent.
More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
TODO: Add other useful fields. apiVersion, kind, uid?
+optional |  |



### <span id="managed-cluster"></span> ManagedCluster


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| apiVersion | string| `string` |  | `"cluster.open-cluster-management.io/v1"`| APIVersion defines the versioned schema of this representation of an object.
Servers should convert recognized schemas to the latest internal value, and
may reject unrecognized values.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
+optional |  |
| kind | string| `string` |  | `"ManagedCluster"`| Kind is a string value representing the REST resource this object represents.
Servers may infer this from the endpoint the client submits requests to.
Cannot be updated.
In CamelCase.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
+optional |  |
| metadata | [Metadata](#metadata)| `Metadata` |  | | metadata for managed cluster. |  |
| spec | [ManagedClusterSpec](#managed-cluster-spec)| `ManagedClusterSpec` |  | | Spec represents a desired configuration for the agent on the managed cluster. |  |
| status | [ManagedClusterStatus](#managed-cluster-status)| `ManagedClusterStatus` |  | | Status represents the current status of joined managed cluster
+optional |  |



### <span id="managed-cluster-claim"></span> ManagedClusterClaim


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| name | string| `string` |  | | Name is the name of a ClusterClaim resource on managed cluster. It's a well known
or customized name to identify the claim.
+kubebuilder:validation:MaxLength=253
+kubebuilder:validation:MinLength=1 |  |
| value | string| `string` |  | | Value is a claim-dependent string
+kubebuilder:validation:MaxLength=1024
+kubebuilder:validation:MinLength=1 |  |



### <span id="managed-cluster-label-patch"></span> ManagedClusterLabelPatch


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| op | string| `string` | ✓ | |  | `add` |
| path | string| `string` | ✓ | |  | `/metadata/labels/foo` |
| value | string| `string` |  | |  | `bar` |



### <span id="managed-cluster-list"></span> ManagedClusterList


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| apiVersion | string| `string` |  | `"cluster.open-cluster-management.io/v1"`| APIVersion defines the versioned schema of this representation of an object.
Servers should convert recognized schemas to the latest internal value, and
may reject unrecognized values.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
+optional |  |
| items | [][ManagedCluster](#managed-cluster)| `[]*ManagedCluster` |  | | Items is a list of managed clusters. |  |
| kind | string| `string` |  | `"ManagedClusterList"`| Kind is a string value representing the REST resource this object represents.
Servers may infer this from the endpoint the client submits requests to.
Cannot be updated.
In CamelCase.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
+optional |  |
| metadata | [ListMetadata](#list-metadata)| `ListMetadata` |  | | Metadata for managed cluster list. |  |



### <span id="managed-cluster-spec"></span> ManagedClusterSpec


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| hubAcceptsClient | boolean| `bool` |  | | hubAcceptsClient represents that hub accepts the joining of Klusterlet agent on
the managed cluster with the hub. The default value is false, and can only be set
true when the user on hub has an RBAC rule to UPDATE on the virtual subresource
of managedclusters/accept.
When the value is set true, a namespace whose name is the same as the name of ManagedCluster
is created on the hub. This namespace represents the managed cluster, also role/rolebinding is created on
the namespace to grant the permision of access from the agent on the managed cluster.
When the value is set to false, the namespace representing the managed cluster is
deleted.
+required |  |
| leaseDurationSeconds | integer| `int64` |  | | LeaseDurationSeconds is used to coordinate the lease update time of Klusterlet agents on the managed cluster.
If its value is zero, the Klusterlet agent will update its lease every 60 seconds by default
+optional
+kubebuilder:default=60 |  |
| managedClusterClientConfigs | [][ClientConfig](#client-config)| `[]*ClientConfig` |  | | ManagedClusterClientConfigs represents a list of the apiserver address of the managed cluster.
If it is empty, the managed cluster has no accessible address for the hub to connect with it.
+optional |  |
| taints | [][Taint](#taint)| `[]*Taint` |  | | Taints is a property of managed cluster that allow the cluster to be repelled when scheduling.
Taints, including 'ManagedClusterUnavailable' and 'ManagedClusterUnreachable', can not be added/removed by agent
running on the managed cluster; while it's fine to add/remove other taints from either hub cluser or managed cluster.
+optional |  |



### <span id="managed-cluster-status"></span> ManagedClusterStatus


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| allocatable | [ResourceList](#resource-list)| `ResourceList` |  | | Allocatable represents the total allocatable resources on the managed cluster. |  |
| capacity | [ResourceList](#resource-list)| `ResourceList` |  | | Capacity represents the total resource capacity from all nodeStatuses
on the managed cluster. |  |
| clusterClaims | [][ManagedClusterClaim](#managed-cluster-claim)| `[]*ManagedClusterClaim` |  | | ClusterClaims represents cluster information that a managed cluster claims,
for example a unique cluster identifier (id.k8s.io) and kubernetes version
(kubeversion.open-cluster-management.io). They are written from the managed
cluster. The set of claims is not uniform across a fleet, some claims can be
vendor or version specific and may not be included from all managed clusters.
+optional |  |
| conditions | [][Condition](#condition)| `[]*Condition` |  | | Conditions contains the different condition statuses for this managed cluster. |  |
| version | [ManagedClusterVersion](#managed-cluster-version)| `ManagedClusterVersion` |  | | Version represents the kubernetes version of the managed cluster. |  |



### <span id="managed-cluster-version"></span> ManagedClusterVersion


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| kubernetes | string| `string` |  | | Kubernetes is the kubernetes version of managed cluster.
+optional |  |



### <span id="metadata"></span> Metadata


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| annotations | map of string| `map[string]string` |  | | Annotations is an unstructured key value map stored with a resource that may be
set by external tools to store and retrieve arbitrary metadata. They are not
queryable and should be preserved when modifying objects.
More info: http://kubernetes.io/docs/user-guide/annotations
+optional |  |
| labels | map of string| `map[string]string` |  | | Map of string keys and values that can be used to organize and categorize
(scope and select) objects. May match selectors of replication controllers
and services.
More info: http://kubernetes.io/docs/user-guide/labels
+optional |  |
| name | string| `string` |  | | Name must be unique within a namespace. Is required when creating resources, although
some resources may allow a client to request the generation of an appropriate name
automatically. Name is primarily intended for creation idempotence and configuration
definition.
Cannot be updated.
More info: http://kubernetes.io/docs/user-guide/identifiers#names
+optional |  |
| namespace | string| `string` |  | | Namespace defines the space within which each name must be unique. An empty namespace is
equivalent to the "default" namespace, but "default" is the canonical representation.
Not all objects are required to be scoped to a namespace - the value of this field for
those objects will be empty.

Must be a DNS_LABEL.
Cannot be updated.
More info: http://kubernetes.io/docs/user-guide/namespaces
+optional |  |
| uid | string| `string` |  | | UID is the unique in time and space value for this object. It is typically generated by
the server on OKful creation of a resource and is not allowed to change on PUT
operations.

Populated by the system.
Read-only.
More info: http://kubernetes.io/docs/user-guide/identifiers#uids
+optional |  |



### <span id="object-meta"></span> ObjectMeta


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| annotations | map of string| `map[string]string` |  | | Annotations is an unstructured key value map stored with a resource that may be
set by external tools to store and retrieve arbitrary metadata. They are not
queryable and should be preserved when modifying objects.
More info: http://kubernetes.io/docs/user-guide/annotations
+optional |  |
| clusterName | string| `string` |  | | Deprecated: ClusterName is a legacy field that was always cleared by
the system and never used; it will be removed completely in 1.25.

The name in the go struct is changed to help clients detect
accidental use.

+optional |  |
| creationTimestamp | string| `string` |  | | CreationTimestamp is a timestamp representing the server time when this object was
created. It is not guaranteed to be set in happens-before order across separate operations.
Clients may not set this value. It is represented in RFC3339 form and is in UTC.

Populated by the system.
Read-only.
Null for lists.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
+optional |  |
| deletionGracePeriodSeconds | integer| `int64` |  | | Number of seconds allowed for this object to gracefully terminate before
it will be removed from the system. Only set when deletionTimestamp is also set.
May only be shortened.
Read-only.
+optional |  |
| deletionTimestamp | string| `string` |  | | DeletionTimestamp is RFC 3339 date and time at which this resource will be deleted. This
field is set by the server when a graceful deletion is requested by the user, and is not
directly settable by a client. The resource is expected to be deleted (no longer visible
from resource lists, and not reachable by name) after the time in this field, once the
finalizers list is empty. As long as the finalizers list contains items, deletion is blocked.
Once the deletionTimestamp is set, this value may not be unset or be set further into the
future, although it may be shortened or the resource may be deleted prior to this time.
For example, a user may request that a pod is deleted in 30 seconds. The Kubelet will react
by sending a graceful termination signal to the containers in the pod. After that 30 seconds,
the Kubelet will send a hard termination signal (SIGKILL) to the container and after cleanup,
remove the pod from the API. In the presence of network partitions, this object may still
exist after this timestamp, until an administrator or automated process can determine the
resource is fully terminated.
If not set, graceful deletion of the object has not been requested.

Populated by the system when a graceful deletion is requested.
Read-only.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
+optional |  |
| finalizers | []string| `[]string` |  | | Must be empty before the object is deleted from the registry. Each entry
is an identifier for the responsible component that will remove the entry
from the list. If the deletionTimestamp of the object is non-nil, entries
in this list can only be removed.
Finalizers may be processed and removed in any order.  Order is NOT enforced
because it introduces significant risk of stuck finalizers.
finalizers is a shared field, any actor with permission can reorder it.
If the finalizer list is processed in order, then this can lead to a situation
in which the component responsible for the first finalizer in the list is
waiting for a signal (field value, external system, or other) produced by a
component responsible for a finalizer later in the list, resulting in a deadlock.
Without enforced ordering finalizers are free to order amongst themselves and
are not vulnerable to ordering changes in the list.
+optional
+patchStrategy=merge |  |
| generateName | string| `string` |  | | GenerateName is an optional prefix, used by the server, to generate a unique
name ONLY IF the Name field has not been provided.
If this field is used, the name returned to the client will be different
than the name passed. This value will also be combined with a unique suffix.
The provided value has the same validation rules as the Name field,
and may be truncated by the length of the suffix required to make the value
unique on the server.

If this field is specified and the generated name exists, the server will return a 409.

Applied only if Name is not specified.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#idempotency
+optional |  |
| generation | integer| `int64` |  | | A sequence number representing a specific generation of the desired state.
Populated by the system. Read-only.
+optional |  |
| labels | map of string| `map[string]string` |  | | Map of string keys and values that can be used to organize and categorize
(scope and select) objects. May match selectors of replication controllers
and services.
More info: http://kubernetes.io/docs/user-guide/labels
+optional |  |
| name | string| `string` |  | | Name must be unique within a namespace. Is required when creating resources, although
some resources may allow a client to request the generation of an appropriate name
automatically. Name is primarily intended for creation idempotence and configuration
definition.
Cannot be updated.
More info: http://kubernetes.io/docs/user-guide/identifiers#names
+optional |  |
| namespace | string| `string` |  | | Namespace defines the space within which each name must be unique. An empty namespace is
equivalent to the "default" namespace, but "default" is the canonical representation.
Not all objects are required to be scoped to a namespace - the value of this field for
those objects will be empty.

Must be a DNS_LABEL.
Cannot be updated.
More info: http://kubernetes.io/docs/user-guide/namespaces
+optional |  |
| ownerReferences | [][OwnerReference](#owner-reference)| `[]*OwnerReference` |  | | List of objects depended by this object. If ALL objects in the list have
been deleted, this object will be garbage collected. If this object is managed by a controller,
then an entry in this list will point to this controller, with the controller field set to true.
There cannot be more than one managing controller.
+optional
+patchMergeKey=uid
+patchStrategy=merge |  |
| resourceVersion | string| `string` |  | | An opaque value that represents the internal version of this object that can
be used by clients to determine when objects have changed. May be used for optimistic
concurrency, change detection, and the watch operation on a resource or set of resources.
Clients must treat these values as opaque and passed unmodified back to the server.
They may only be valid for a particular resource or set of resources.

Populated by the system.
Read-only.
Value must be treated as opaque by clients and .
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
+optional |  |
| selfLink | string| `string` |  | | Deprecated: selfLink is a legacy read-only field that is no longer populated by the system.
+optional |  |
| uid | string| `string` |  | | UID is the unique in time and space value for this object. It is typically generated by
the server on OKful creation of a resource and is not allowed to change on PUT
operations.

Populated by the system.
Read-only.
More info: http://kubernetes.io/docs/user-guide/identifiers#uids
+optional |  |



### <span id="object-reference"></span> ObjectReference


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| apiVersion | string| `string` |  | | API version of the referent.
+optional |  |
| fieldPath | string| `string` |  | | If referring to a piece of an object instead of an entire object, this string
should contain a valid JSON/Go field access statement, such as desiredState.manifest.containers[2].
For example, if the object reference is to a container within a pod, this would take on a value like:
"spec.containers{name}" (where "name" refers to the name of the container that triggered
the event) or if no container name is specified "spec.containers[2]" (container with
index 2 in this pod). This syntax is chosen only to have some well-defined way of
referencing a part of an object.
TODO: this design is not final and this field is subject to change in the future.
+optional |  |
| kind | string| `string` |  | | Kind of the referent.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
+optional |  |
| name | string| `string` |  | | Name of the referent.
More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
+optional |  |
| namespace | string| `string` |  | | Namespace of the referent.
More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
+optional |  |
| resourceVersion | string| `string` |  | | Specific resourceVersion to which this reference is made, if any.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
+optional |  |
| uid | string| `string` |  | | UID of the referent.
More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#uids
+optional |  |



### <span id="overrides"></span> Overrides


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| packageAlias | string| `string` |  | |  |  |
| packageName | string| `string` |  | |  |  |
| packageOverrides | [][PackageOverride](#package-override)| `[]PackageOverride` |  | | To be added |  |



### <span id="owner-reference"></span> OwnerReference


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| apiVersion | string| `string` |  | | API version of the referent. |  |
| blockOwnerDeletion | boolean| `bool` |  | | If true, AND if the owner has the "foregroundDeletion" finalizer, then
the owner cannot be deleted from the key-value store until this
reference is removed.
See https://kubernetes.io/docs/concepts/architecture/garbage-collection/#foreground-deletion
for how the garbage collector interacts with this field and enforces the foreground deletion.
Defaults to false.
To set this field, a user needs "delete" permission of the owner,
otherwise 422 (Unprocessable Entity) will be returned.
+optional |  |
| controller | boolean| `bool` |  | | If true, this reference points to the managing controller.
+optional |  |
| kind | string| `string` |  | | Kind of the referent.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |
| name | string| `string` |  | | Name of the referent.
More info: http://kubernetes.io/docs/user-guide/identifiers#names |  |
| uid | string| `string` |  | | UID of the referent.
More info: http://kubernetes.io/docs/user-guide/identifiers#uids |  |



### <span id="package-filter"></span> PackageFilter


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| annotations | map of string| `map[string]string` |  | |  |  |
| filterRef | [LocalObjectReference](#local-object-reference)| `LocalObjectReference` |  | |  |  |
| labelSelector | [LabelSelector](#label-selector)| `LabelSelector` |  | |  |  |
| version | string| `string` |  | | +kubebuilder:validation:Pattern=([0-9]+)((\.[0-9]+)(\.[0-9]+)|(\.[0-9]+)?(\.[xX]))$ |  |



### <span id="package-override"></span> PackageOverride


  

[interface{}](#interface)

### <span id="placement"></span> Placement


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| decisions | [][PlacementDecision](#placement-decision)| `[]*PlacementDecision` |  | |  |  |
| placement | string| `string` |  | |  |  |
| placementBinding | string| `string` |  | |  |  |
| placementRule | string| `string` |  | |  |  |
| policySet | string| `string` |  | |  |  |



### <span id="placement-decision"></span> PlacementDecision


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| clusterName | string| `string` |  | |  |  |
| clusterNamespace | string| `string` |  | |  |  |



### <span id="policy"></span> Policy


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| apiVersion | string| `string` |  | `"policy.open-cluster-management.io/v1"`| APIVersion defines the versioned schema of this representation of an object.
Servers should convert recognized schemas to the latest internal value, and
may reject unrecognized values.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
+optional |  |
| kind | string| `string` |  | `"Policy"`| Kind is a string value representing the REST resource this object represents.
Servers may infer this from the endpoint the client submits requests to.
Cannot be updated.
In CamelCase.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
+optional |  |
| metadata | [Metadata](#metadata)| `Metadata` |  | | metadata for policy. |  |
| spec | [interface{}](#interface)| `interface{}` |  | |  |  |
| status | [PolicyStatus](#policy-status)| `PolicyStatus` |  | |  |  |



### <span id="policy-list"></span> PolicyList


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| apiVersion | string| `string` |  | `"policy.open-cluster-management.io/v1"`| APIVersion defines the versioned schema of this representation of an object.
Servers should convert recognized schemas to the latest internal value, and
may reject unrecognized values.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
+optional |  |
| items | [][Policy](#policy)| `[]*Policy` |  | |  |  |
| kind | string| `string` |  | `"PolicyList"`| Kind is a string value representing the REST resource this object represents.
Servers may infer this from the endpoint the client submits requests to.
Cannot be updated.
In CamelCase.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
+optional |  |
| metadata | [ListMetadata](#list-metadata)| `ListMetadata` |  | | Metadata for policy list. |  |



### <span id="policy-status"></span> PolicyStatus


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| compliant | string| `string` |  | | +kubebuilder:validation:Enum=Compliant;NonCompliant |  |
| details | [][DetailsPerTemplate](#details-per-template)| `[]*DetailsPerTemplate` |  | | used by replicated policy |  |
| placement | [][Placement](#placement)| `[]*Placement` |  | | used by root policy |  |
| status | [][CompliancePerClusterStatus](#compliance-per-cluster-status)| `[]*CompliancePerClusterStatus` |  | | used by root policy |  |
| summary | [PolicySummary](#policy-summary)| `PolicySummary` |  | | policy compliance summry information |  |



### <span id="policy-summary"></span> PolicySummary


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| complianceClusterNumber | integer| `int64` |  | | number of compliant managed clusters |  |
| nonComplianceClusterNumber | integer| `int64` |  | | number of non-compliant managed clusters |  |



### <span id="resource-list"></span> ResourceList


  

[ResourceList](#resource-list)

### <span id="subscription"></span> Subscription


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| apiVersion | string| `string` |  | `"apps.open-cluster-management.io/v1"`| APIVersion defines the versioned schema of this representation of an object.
Servers should convert recognized schemas to the latest internal value, and
may reject unrecognized values.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
+optional |  |
| kind | string| `string` |  | `"Subscription"`| Kind is a string value representing the REST resource this object represents.
Servers may infer this from the endpoint the client submits requests to.
Cannot be updated.
In CamelCase.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
+optional |  |
| metadata | [Metadata](#metadata)| `Metadata` |  | | metadata for subscription. |  |
| spec | [SubscriptionSpec](#subscription-spec)| `SubscriptionSpec` |  | |  |  |
| status | [SubscriptionStatus](#subscription-status)| `SubscriptionStatus` |  | |  |  |



### <span id="subscription-cluster-status-map"></span> SubscriptionClusterStatusMap


  

[SubscriptionClusterStatusMap](#subscription-cluster-status-map)

### <span id="subscription-list"></span> SubscriptionList


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| apiVersion | string| `string` |  | `"apps.open-cluster-management.io/v1"`| APIVersion defines the versioned schema of this representation of an object.
Servers should convert recognized schemas to the latest internal value, and
may reject unrecognized values.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
+optional |  |
| items | [][Subscription](#subscription)| `[]*Subscription` |  | |  |  |
| kind | string| `string` |  | `"SubscriptionList"`| Kind is a string value representing the REST resource this object represents.
Servers may infer this from the endpoint the client submits requests to.
Cannot be updated.
In CamelCase.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
+optional |  |
| metadata | [ListMetadata](#list-metadata)| `ListMetadata` |  | | Metadata for subscription list. |  |



### <span id="subscription-per-cluster-status"></span> SubscriptionPerClusterStatus


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| packages | map of [SubscriptionUnitStatus](#subscription-unit-status)| `map[string]SubscriptionUnitStatus` |  | |  |  |



### <span id="subscription-report"></span> SubscriptionReport


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| apiVersion | string| `string` |  | `"apps.open-cluster-management.io/v1alpha1"`| APIVersion defines the versioned schema of this representation of an object.
Servers should convert recognized schemas to the latest internal value, and
may reject unrecognized values.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
+optional |  |
| kind | string| `string` |  | `"SubscriptionReport"`| Kind is a string value representing the REST resource this object represents.
Servers may infer this from the endpoint the client submits requests to.
Cannot be updated.
In CamelCase.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
+optional |  |
| metadata | [Metadata](#metadata)| `Metadata` |  | | metadata for subscription report. |  |
| reportType | string| `string` |  | | ReportType is the name or identifier of the type of report |  |
| resources | [][ObjectReference](#object-reference)| `[]*ObjectReference` |  | | Resources is an optional reference to the subscription resources
+optional |  |
| results | [][SubscriptionReportResult](#subscription-report-result)| `[]*SubscriptionReportResult` |  | | SubscriptionReportResult provides result details
+optional |  |
| summary | [SubscriptionReportSummary](#subscription-report-summary)| `SubscriptionReportSummary` |  | | SubscriptionReportSummary provides a summary of results
+optional |  |



### <span id="subscription-report-result"></span> SubscriptionReportResult


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| result | string| `string` |  | | Result indicates the outcome of the subscription deployment |  |
| source | string| `string` |  | | Source is an identifier for the subscription
+optional |  |
| timestamp | [Timestamp](#timestamp)| `Timestamp` |  | | Timestamp indicates the time the result was found |  |



### <span id="subscription-report-summary"></span> SubscriptionReportSummary


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| clusters | string| `string` |  | | Clusters provides the count of all managed clusters the subscription is deployed to
+optional |  |
| deployed | string| `string` |  | | Deployed provides the count of subscriptions that deployed successfully
+optional |  |
| failed | string| `string` |  | | Failed provides the count of subscriptions that failed to deploy
+optional |  |
| inProgress | string| `string` |  | | InProgress provides the count of subscriptions that are in the process of being deployed
+optional |  |
| propagationFailed | string| `string` |  | | PropagationFailed provides the count of subscriptions that failed to propagate to a managed cluster
+optional |  |



### <span id="subscription-spec"></span> SubscriptionSpec


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| allow | [][AllowDenyItem](#allow-deny-item)| `[]*AllowDenyItem` |  | |  |  |
| channel | string| `string` |  | |  |  |
| deny | [][AllowDenyItem](#allow-deny-item)| `[]*AllowDenyItem` |  | |  |  |
| hooksecretref | [ObjectReference](#object-reference)| `ObjectReference` |  | | +optional |  |
| name | string| `string` |  | | To specify 1 package in channel |  |
| overrides | [][ClusterOverrides](#cluster-overrides)| `[]*ClusterOverrides` |  | | for hub use only to specify the overrides when apply to clusters |  |
| packageFilter | [PackageFilter](#package-filter)| `PackageFilter` |  | | To specify more than 1 package in channel |  |
| packageOverrides | [][Overrides](#overrides)| `[]*Overrides` |  | | To provide flexibility to override package in channel with local input |  |
| placement | [PlacementruleV1Placement](#placementrule-v1-placement)| `PlacementruleV1Placement` |  | | For hub use only, to specify which clusters to go to |  |
| secondaryChannel | string| `string` |  | | When fails to connect to the channel, connect to the secondary channel |  |
| timewindow | [TimeWindow](#time-window)| `TimeWindow` |  | | help user control when the subscription will take affect |  |
| watchHelmNamespaceScopedResources | boolean| `bool` |  | | WatchHelmNamespaceScopedResources is used to enable watching namespace scope Helm chart resources |  |



### <span id="subscription-status"></span> SubscriptionStatus


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| ansiblejobs | [AnsibleJobsStatus](#ansible-jobs-status)| `AnsibleJobsStatus` |  | | +optional |  |
| appstatusReference | string| `string` |  | |  |  |
| lastUpdateTime | string| `string` |  | |  |  |
| message | string| `string` |  | |  |  |
| phase | string| `string` |  | | INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
Important: Run "make" to regenerate code after modifying this file |  |
| reason | string| `string` |  | |  |  |
| statuses | [SubscriptionClusterStatusMap](#subscription-cluster-status-map)| `SubscriptionClusterStatusMap` |  | | For endpoint, it is the status of subscription, key is packagename,
For hub, it aggregates all status, key is cluster name |  |



### <span id="subscription-unit-status"></span> SubscriptionUnitStatus


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| lastUpdateTime | string| `string` |  | |  |  |
| message | string| `string` |  | |  |  |
| phase | string| `string` |  | | Phase are Propagated if it is in hub or Subscribed if it is in endpoint |  |
| reason | string| `string` |  | |  |  |
| resourceStatus | [RuntimeRawExtension](#runtime-raw-extension)| `RuntimeRawExtension` |  | |  |  |



### <span id="taint"></span> Taint


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| effect | string| `string` |  | | Effect indicates the effect of the taint on placements that do not tolerate the taint.
Valid effects are NoSelect, PreferNoSelect and NoSelectIfNew.
+kubebuilder:validation:Required
+kubebuilder:validation:Enum:=NoSelect;PreferNoSelect;NoSelectIfNew
+required |  |
| key | string| `string` |  | | Key is the taint key applied to a cluster. e.g. bar or foo.example.com/bar.
The regex it matches is (dns1123SubdomainFmt/)?(qualifiedNameFmt)
+kubebuilder:validation:Required
+kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
+kubebuilder:validation:MaxLength=316
+required |  |
| timeAdded | string| `string` |  | | TimeAdded represents the time at which the taint was added.
+nullable
+required |  |
| value | string| `string` |  | | Value is the taint value corresponding to the taint key.
+kubebuilder:validation:MaxLength=1024
+optional |  |



### <span id="time-window"></span> TimeWindow


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| daysofweek | []string| `[]string` |  | | weekdays defined the day of the week for this time window https://golang.org/pkg/time/#Weekday |  |
| hours | [][HourRange](#hour-range)| `[]*HourRange` |  | |  |  |
| location | string| `string` |  | | https://en.wikipedia.org/wiki/List_of_tz_database_time_zones |  |
| windowtype | string| `string` |  | | active time window or not, if timewindow is active, then deploy will only applies during these windows
Note, if you want to generation crd with operator-sdk v0.10.0, then the following line should be:
<+kubebuilder:validation:Enum=active,blocked,Active,Blocked>
+kubebuilder:validation:Enum={active,blocked,Active,Blocked} |  |



### <span id="timestamp"></span> Timestamp


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| nanos | integer| `int64` |  | | Non-negative fractions of a second at nanosecond resolution. Negative
second values with fractions must still have non-negative nanos values
that count forward in time. Must be from 0 to 999,999,999
inclusive. This field may be limited in precision depending on context. |  |
| seconds | integer| `int64` |  | | Represents seconds of UTC time since Unix epoch
1970-01-01T00:00:00Z. Must be from 0001-01-01T00:00:00Z to
9999-12-31T23:59:59Z inclusive. |  |



### <span id="placementrule-v1-placement"></span> placementrule_v1_Placement


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| clusterSelector | [LabelSelector](#label-selector)| `LabelSelector` |  | |  |  |
| clusters | [][GenericClusterReference](#generic-cluster-reference)| `[]*GenericClusterReference` |  | |  |  |
| local | boolean| `bool` |  | |  |  |
| placementRef | [ObjectReference](#object-reference)| `ObjectReference` |  | |  |  |



### <span id="resource-quantity"></span> resource.Quantity


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| Format | string| `string` |  | |  |  |



### <span id="runtime-raw-extension"></span> runtime.RawExtension


  

[interface{}](#interface)
