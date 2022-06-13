// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer

// API Info.
const (
	appsv1APIGroup         = "apps.open-cluster-management.io/v1"
	clusterv1beta1APIGroup = "cluster.open-cluster-management.io/v1beta1"
	placementKind          = "Placement"
	subscriptionKind       = "Subscription"
)

// DB Tables.
const (
	placementsSpecTableName   = "placements"
	placementsStatusTableName = "placements"

	placementDecisionsStatusTableName = "placementdecisions"

	subscriptionsSpecTableName = "subscriptions"

	subscriptionStatusesTableName      = "subscription_statuses"
	subscriptionReportsStatusTableName = "subscription_reports"
)
