// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

const (
	ServerInternalErrorMsg                = "internal error"
	QueryPolicyFailureFormatMsg           = "error in querying policy: %v\n"
	QueryPoliciesFailureFormatMsg         = "error in querying policies: %v\n"
	QueryPolicyComplianceFailureFormatMsg = "error in querying compliance status of a policy with UID: %v\n"
	QueryPolicyMappingFailureFormatMsg    = "error in querying policy&placementbinding&placementrule mapping: %v\n"
)

const (
	policyQuery = `SELECT payload FROM spec.policies WHERE deleted = FALSE AND id=$1
		ORDER BY payload -> 'metadata' ->> 'name'`
	policiesQuery = `SELECT id, payload FROM spec.policies WHERE deleted = FALSE
		ORDER BY payload -> 'metadata' ->> 'name'`
	policyComplianceQuery = `SELECT cluster_name,leaf_hub_name,compliance FROM status.compliance
		WHERE id=$1 ORDER BY leaf_hub_name, cluster_name`
	policyMappingQuery = `SELECT p.payload -> 'metadata' ->> 'name' AS policy,
								 pb.payload -> 'metadata' ->> 'name' AS binding,
								 pr.payload -> 'metadata' ->> 'name' AS placementrule
							FROM spec.policies p
						INNER JOIN spec.placementbindings pb ON 
							p.payload -> 'metadata' ->> 'namespace' = pb.payload -> 'metadata' ->> 'namespace'
							AND
							pb.payload -> 'subjects' @> json_build_array(json_build_object(
								'name', p.payload -> 'metadata' ->> 'name',
								'kind', p.payload ->> 'kind',
								'apiGroup', split_part(p.payload ->> 'apiVersion', '/',1)
								))::jsonb
						INNER JOIN spec.placementrules pr ON
							pr.payload -> 'metadata' ->> 'namespace' = pb.payload -> 'metadata' ->> 'namespace'
							AND
							pr.payload -> 'metadata' ->> 'name' = pb.payload -> 'placementRef' ->> 'name'
							AND
							pr.payload ->> 'kind' = pb.payload -> 'placementRef' ->> 'kind'
							AND
							split_part(pr.payload ->> 'apiVersion', '/', 1) = pb.payload -> 'placementRef' ->> 'apiGroup'`
)

const (
	syncIntervalInSeconds = 4
	crdName               = "policies.policy.open-cluster-management.io"
)

const (
	dbEnumCompliant    = "compliant"
	dbEnumNonCompliant = "non_compliant"
)
