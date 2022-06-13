// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/db"
	"k8s.io/apimachinery/pkg/api/errors"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	dbEnumCompliant    = "compliant"
	dbEnumNonCompliant = "non_compliant"

	policiesSpecTableName     = "policies"
	complianceStatusTableName = "compliance"
)

func AddPolicyDBSyncer(mgr ctrl.Manager, database db.DB, statusSyncInterval time.Duration) error {
	err := mgr.Add(&genericDBSyncer{
		statusSyncInterval: statusSyncInterval,
		statusSyncFunc: func(ctx context.Context) {
			syncPolicies(ctx,
				ctrl.Log.WithName("policies-db-syncer"),
				database, mgr.GetClient(),
				map[string]policyv1.ComplianceState{
					dbEnumCompliant:    policyv1.Compliant,
					dbEnumNonCompliant: policyv1.NonCompliant,
				})
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add policies status syncer to the manager: %w", err)
	}

	return nil
}

func syncPolicies(ctx context.Context, log logr.Logger, database db.DB, k8sClient client.Client,
	dbEnumToPolicyComplianceStateMap map[string]policyv1.ComplianceState,
) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT id, payload->'metadata'->>'name', payload->'metadata'->>'namespace' 
		FROM spec.%s WHERE deleted = FALSE`, policiesSpecTableName))
	if err != nil {
		log.Error(err, "error in getting policies spec")
		return
	}

	for rows.Next() {
		var id, name, namespace string

		err := rows.Scan(&id, &name, &namespace)
		if err != nil {
			log.Error(err, "error in select", "table", policiesSpecTableName)
			continue
		}

		instance := &policyv1.Policy{}

		err = k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, instance)
		if err != nil {
			log.Error(err, "error in getting CR", "name", name, "namespace", namespace)
			continue
		}

		go handlePolicy(ctx, log, database, k8sClient, dbEnumToPolicyComplianceStateMap, instance)
	}
}

func handlePolicy(ctx context.Context, log logr.Logger, database db.DB,
	k8sClient client.StatusClient, dbEnumToPolicyComplianceStateMap map[string]policyv1.ComplianceState,
	policy *policyv1.Policy,
) {
	compliancePerClusterStatuses, hasNonCompliantClusters, err := getComplianceStatus(ctx, database,
		dbEnumToPolicyComplianceStateMap, policy)
	if err != nil {
		log.Error(err, "failed to get compliance status of a policy", "uid", policy.GetUID())
		return
	}

	if err = updateComplianceStatus(ctx, k8sClient, policy, compliancePerClusterStatuses,
		hasNonCompliantClusters); err != nil {
		log.Error(err, "failed to update policy status")
	}
}

// returns array of CompliancePerClusterStatus, whether the policy has any NonCompliant cluster, and error.
func getComplianceStatus(ctx context.Context, database db.DB,
	dbEnumToPolicyComplianceStateMap map[string]policyv1.ComplianceState,
	policy *policyv1.Policy,
) ([]*policyv1.CompliancePerClusterStatus, bool, error) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT cluster_name,leaf_hub_name,compliance FROM status.%s
			WHERE id=$1 ORDER BY leaf_hub_name, cluster_name`, complianceStatusTableName), string(policy.GetUID()))
	if err != nil {
		return []*policyv1.CompliancePerClusterStatus{}, false,
			fmt.Errorf("error in getting policy compliance statuses from DB - %w", err)
	}

	defer rows.Close()

	var compliancePerClusterStatuses []*policyv1.CompliancePerClusterStatus

	hasNonCompliantClusters := false

	for rows.Next() {
		var clusterName, leafHubName, complianceInDB string

		if err := rows.Scan(&clusterName, &leafHubName, &complianceInDB); err != nil {
			return []*policyv1.CompliancePerClusterStatus{}, false,
				fmt.Errorf("error in getting policy compliance statuses from DB - %w", err)
		}

		compliance := dbEnumToPolicyComplianceStateMap[complianceInDB]

		if compliance == policyv1.NonCompliant {
			hasNonCompliantClusters = true
		}

		compliancePerClusterStatuses = append(compliancePerClusterStatuses, &policyv1.CompliancePerClusterStatus{
			ComplianceState:  compliance,
			ClusterName:      clusterName,
			ClusterNamespace: clusterName,
		})
	}

	return compliancePerClusterStatuses, hasNonCompliantClusters, nil
}

func updateComplianceStatus(ctx context.Context, k8sClient client.StatusClient, policy *policyv1.Policy,
	compliancePerClusterStatuses []*policyv1.CompliancePerClusterStatus,
	hasNonCompliantClusters bool,
) error {
	originalPolicy := policy.DeepCopy()

	policy.Status.Status = compliancePerClusterStatuses
	policy.Status.ComplianceState = ""

	if hasNonCompliantClusters {
		policy.Status.ComplianceState = policyv1.NonCompliant
	} else if len(compliancePerClusterStatuses) > 0 {
		policy.Status.ComplianceState = policyv1.Compliant
	}

	err := k8sClient.Status().Patch(ctx, policy, client.MergeFrom(originalPolicy))
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to update policy CR: %w", err)
	}

	return nil
}
