

function limit_range_policy() {
plicy_name=$1
echo "Creating Simulated Rootpolicy $plicy_name..."
cat <<EOF | kubectl apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  annotations:
    policy.open-cluster-management.io/categories: PR.IP Information Protection Processes and Procedures
    policy.open-cluster-management.io/controls: PR.IP-1 Baseline Configuration
    policy.open-cluster-management.io/standards: NIST-CSF
  name: $plicy_name
  namespace: default
spec:
  disabled: false
  policy-templates:
  - objectDefinition:
      apiVersion: policy.open-cluster-management.io/v1
      kind: ConfigurationPolicy
      metadata:
        name: policy-limitrange-container-mem-limit-range
      spec:
        namespaceSelector:
          exclude:
          - kube-*
          include:
          - default
        object-templates:
        - complianceType: musthave
          objectDefinition:
            apiVersion: v1
            kind: LimitRange
            metadata:
              name: container-mem-limit-range
            spec:
              limits:
              - default:
                  memory: 512Mi
                defaultRequest:
                  memory: 256Mi
                type: Container
        remediationAction: inform
        severity: medium
  remediationAction: inform
status:
  compliant: NonCompliant
  placement:
  - placement: placement-policy-limitrange
    placementBinding: binding-policy-limitrange
EOF
}

function limit_range_replicas_policy() {
root_policy_namespace=$1
root_plicy_name=$2
cluster_name=$3
echo "Creating Simulated ReplicasPolicy $root_policy_namespace.$root_plicy_name on $cluster_name..."
kubectl create ns $cluster_name --dry-run=client -oyaml | kubectl apply -f -
cat <<EOF | kubectl apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  annotations:
    policy.open-cluster-management.io/categories: PR.IP Information Protection Processes and Procedures
    policy.open-cluster-management.io/controls: PR.IP-1 Baseline Configuration
    policy.open-cluster-management.io/standards: NIST-CSF
  labels:
    policy.open-cluster-management.io/cluster-name: $cluster_name
    policy.open-cluster-management.io/cluster-namespace: $cluster_name
    policy.open-cluster-management.io/root-policy: $root_policy_namespace.$root_plicy_name
  name: $root_policy_namespace.$root_plicy_name
  namespace: $cluster_name
spec:
  disabled: false
  policy-templates:
  - objectDefinition:
      apiVersion: policy.open-cluster-management.io/v1
      kind: ConfigurationPolicy
      metadata:
        name: policy-limitrange-container-mem-limit-range
      spec:
        namespaceSelector:
          exclude:
          - kube-*
          include:
          - default
        object-templates:
        - complianceType: musthave
          objectDefinition:
            apiVersion: v1
            kind: LimitRange
            metadata:
              name: container-mem-limit-range
            spec:
              limits:
              - default:
                  memory: 512Mi
                defaultRequest:
                  memory: 256Mi
                type: Container
        remediationAction: inform
        severity: medium
  remediationAction: inform
status:
  compliant: NonCompliant
  details:
  - compliant: NonCompliant
    history:
    - eventName: $root_policy_namespace.$root_plicy_name.177fbbf4ad21c647
      lastTimestamp: "2023-08-29T03:10:44Z"
      message: NonCompliant; violation - limitranges [container-mem-limit-range] not found in namespace $root_policy_namespace
    templateMeta:
      creationTimestamp: null
      name: policy-limitrange-container-mem-limit-range
EOF

# patch replicas policy status
kubectl patch policy $root_policy_namespace.$root_plicy_name -n $cluster_name --type=merge --subresource status --patch "status: {compliant: NonCompliant, details: [{compliant: NonCompliant, history: [{eventName: $root_policy_namespace.$root_plicy_name.$cluster_name, message: NonCompliant; violation - limitranges container-mem-limit-range not found in namespace $root_policy_namespace}], templateMeta: {creationTimestamp: null, name: policy-limitrange-container-mem-limit-range}}]}"
}

function generate_placement() {
placement_namespace="$1"
placement_name="$2"
decsion="$3"

cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: $placement_name
  namespace: $placement_namespace
spec:
  clusterSets:
  - global
EOF

kubectl patch placement $placement_name -n $placement_namespace --type=merge --subresource status --patch 'status: {conditions: [{lastTransitionTime: 2023-08-29T03:38:43Z, message: Placement configurations check pass, reason: Succeedconfigured, status: "False", type: PlacementMisconfigured}, {lastTransitionTime: 2023-08-29T03:38:43Z, message: All cluster decisions scheduled, reason: AllDecisionsScheduled, status: "True", type: PlacementSatisfied}], numberOfSelectedClusters: 1}'

palcementId=$(kubectl get placement $placement_name -n $placement_namespace -o jsonpath='{.metadata.uid}')

cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: PlacementDecision
metadata:
  labels:
    cluster.open-cluster-management.io/placement: $placement_name
  name: $placement_name-1
  namespace: $placement_namespace
  ownerReferences:
  - apiVersion: cluster.open-cluster-management.io/v1beta1
    blockOwnerDeletion: true
    controller: true
    kind: Placement
    name: $placement_name
    uid: $palcementId
EOF

kubectl patch placementdecision $placement_name-1 -n $placement_namespace --type=merge --subresource status --patch "status: {decisions: [${decision}]}"
}