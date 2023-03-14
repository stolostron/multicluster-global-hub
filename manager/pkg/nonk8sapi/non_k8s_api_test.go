// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package nonk8sapi_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
)

type TestResponseRecorder struct {
	*httptest.ResponseRecorder
	closeChannel chan bool
}

func (r *TestResponseRecorder) CloseNotify() <-chan bool {
	return r.closeChannel
}

func (r *TestResponseRecorder) closeClient() {
	r.closeChannel <- true
}

func CreateTestResponseRecorder() *TestResponseRecorder {
	return &TestResponseRecorder{
		httptest.NewRecorder(),
		make(chan bool, 1),
	}
}

var _ = Describe("Nonk8s API Server", Ordered, func() {
	var postgresSQL *postgresql.PostgreSQL
	var router *gin.Engine
	var plc1ID string
	var sub1ID string
	var sub2ID string

	BeforeAll(func() {
		var err error

		By("Create connection to the database")
		dataConfig := &config.DatabaseConfig{
			ProcessDatabaseURL: testPostgres.URI,
			CACertPath:         "ca-cert-path",
		}
		postgresSQL, err = postgresql.NewSpecPostgreSQL(context.TODO(), dataConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(postgresSQL).NotTo(BeNil())

		By("Create test tables in the database")
		_, err = postgresSQL.GetConn().Exec(ctx, `
			CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
			CREATE SCHEMA IF NOT EXISTS spec;
			CREATE SCHEMA IF NOT EXISTS status;

			DO $$ BEGIN
				CREATE TYPE status.compliance_type AS ENUM (
					'compliant',
					'non_compliant',
					'unknown'
				);
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;

			DO $$ BEGIN
				CREATE TYPE status.error_type AS ENUM (
					'disconnected',
					'none'
				);
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;

			CREATE TABLE IF NOT EXISTS status.managed_clusters (
				leaf_hub_name character varying(63) NOT NULL,
				payload jsonb NOT NULL,
				error status.error_type NOT NULL
			);
			CREATE TABLE IF NOT EXISTS spec.managed_clusters_labels (
				id uuid NOT NULL,
				leaf_hub_name character varying(63) DEFAULT ''::character varying NOT NULL,
				managed_cluster_name character varying(63) NOT NULL,
				labels jsonb DEFAULT '{}'::jsonb NOT NULL,
				deleted_label_keys jsonb DEFAULT '[]'::jsonb NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				version bigint DEFAULT 0 NOT NULL,
				CONSTRAINT managed_clusters_labels_version_check CHECK ((version >= 0))
			);
			CREATE TABLE IF NOT EXISTS spec.policies (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
			CREATE TABLE IF NOT EXISTS spec.placementrules (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
			CREATE TABLE IF NOT EXISTS spec.placementbindings (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
			CREATE TABLE IF NOT EXISTS status.placementrules (
				id uuid NOT NULL,
				leaf_hub_name character varying(63) NOT NULL,
				payload jsonb NOT NULL
			);
			CREATE TABLE IF NOT EXISTS status.compliance (
				id uuid NOT NULL,
				cluster_name character varying(63) NOT NULL,
				leaf_hub_name character varying(63) NOT NULL,
				error status.error_type NOT NULL,
				compliance status.compliance_type NOT NULL
			);
			CREATE TABLE IF NOT EXISTS spec.subscriptions (
				id uuid NOT NULL,
				payload jsonb NOT NULL,
				created_at timestamp without time zone DEFAULT now() NOT NULL,
				updated_at timestamp without time zone DEFAULT now() NOT NULL,
				deleted boolean DEFAULT false NOT NULL
			);
			CREATE TABLE IF NOT EXISTS status.subscription_reports (
				id uuid NOT NULL,
				leaf_hub_name character varying(63) NOT NULL,
				payload jsonb NOT NULL
			);
		`)
		Expect(err).ToNot(HaveOccurred())

		By("Set up nonk8s-api server router")
		router, err = nonk8sapi.SetupRouter(postgresSQL, &nonk8sapi.NonK8sAPIServerConfig{
			ServerBasePath: "/global-hub-api/v1",
			ClusterAPIURL:  testAuthServer.URL,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("Should be able to list managed clusters", func() {
		hub1, mc1, mc2 := "hub1", `
{
	"kind": "ManagedCluster",
	"apiVersion": "cluster.open-cluster-management.io/v1",
	"metadata": {
		"uid": "2aa5547c-c172-47ed-b70b-db468c84d327",
		"creationTimestamp": null,
		"name": "mc1",
		"labels": {
			"cloud": "Other",
			"vendor": "Other"
		},
		"annotations": {
			"global-hub.open-cluster-management.io/managed-by": "hub1",
			"open-cluster-management/created-via": "other"
		}
	},
	"spec": {
		"hubAcceptsClient": true,
		"leaseDurationSeconds": 60
	},
	"status": {
		"conditions": null,
		"version": {}
	}
}
`, `
{
	"kind": "ManagedCluster",
	"apiVersion": "cluster.open-cluster-management.io/v1",
	"metadata": {
		"uid": "18c9e13c-4488-4dcd-a5ac-1196093abbc0",
		"creationTimestamp": null,
		"name": "mc2",
		"labels": {
			"cloud": "Other",
			"vendor": "Other"
		},
		"annotations": {
			"global-hub.open-cluster-management.io/managed-by": "hub1",
			"open-cluster-management/created-via": "other"
		}
	},
	"spec": {
		"hubAcceptsClient": true,
		"leaseDurationSeconds": 60
	},
	"status": {
		"conditions": null,
		"version": {}
	}
}
`

		By("Check the empty managedcclusters can be listed without parameters")
		w0 := httptest.NewRecorder()
		req0, err := http.NewRequest("GET", "/global-hub-api/v1/managedclusters", nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w0, req0)
		Expect(w0.Code).To(Equal(200))
		managedClusterListFormatStr := `
		{
		"kind": "ManagedClusterList",
		"apiVersion": "cluster.open-cluster-management.io/v1",
		"metadata": {},
		"items": []
		}`
		Expect(w0.Body.String()).Should(MatchJSON(managedClusterListFormatStr))

		By("Insert testing managed clusters")
		_, err = postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO status.managed_clusters (leaf_hub_name,payload,error) VALUES ($1, $2, 'none');`,
			hub1,
			mc1)
		Expect(err).ToNot(HaveOccurred())
		_, err = postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO status.managed_clusters (leaf_hub_name,payload,error) VALUES ($1, $2, 'none');`,
			hub1,
			mc2)
		Expect(err).ToNot(HaveOccurred())

		By("Check the managedcclusters can be listed without parameters")
		w1 := httptest.NewRecorder()
		req1, err := http.NewRequest("GET", "/global-hub-api/v1/managedclusters", nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w1, req1)
		Expect(w1.Code).To(Equal(200))
		managedClusterListFormatStr = `
{
	"kind": "ManagedClusterList",
	"apiVersion": "cluster.open-cluster-management.io/v1",
	"metadata": {},
	"items": [
		%s,
		%s
		]
}`
		Expect(w1.Body.String()).Should(MatchJSON(
			fmt.Sprintf(managedClusterListFormatStr, mc1, mc2)))

		By("Check the managedcclusters can be listed with limit and labelSelector")
		w2 := httptest.NewRecorder()
		req2, err := http.NewRequest("GET",
			"/global-hub-api/v1/managedclusters?"+
				"limit=2&labelSelector=cloud%3DOther%2Cvendor%21%3DOpenshift%2C%21testnokey%2Cvendor",
			nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w2, req2)
		Expect(w2.Code).To(Equal(200))
		Expect(w2.Body.String()).Should(MatchJSON(
			fmt.Sprintf(managedClusterListFormatStr, mc1, mc2)))

		By("Check the managedcclusters can be listed as table")
		mclTable := `
{
	"kind": "Table",
	"apiVersion": "meta.k8s.io/v1",
	"metadata": {},
	"columnDefinitions": [
		{
		"name": "Name",
		"type": "string",
		"format": "name",
		"description": "Name must be unique within a namespace. Is required when creating resources, although some resources may allow a client to request the generation of an appropriate name automatically. Name is primarily intended for creation idempotence and configuration definition. Cannot be updated. More info: http://kubernetes.io/docs/user-guide/identifiers#names",
		"priority": 0
		},
		{
		"name": "Age",
		"type": "date",
		"format": "",
		"description": "Custom resource definition column (in JSONPath format): .metadata.creationTimestamp",
		"priority": 0
		}
	],
	"rows": [
		{
		"cells": [
			"mc1",
			null
		],
		"object": {
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind": "ManagedCluster",
			"metadata": {
			"uid": "2aa5547c-c172-47ed-b70b-db468c84d327",
			"annotations": {
				"global-hub.open-cluster-management.io/managed-by": "hub1",
				"open-cluster-management/created-via": "other"
			},
			"creationTimestamp": null,
			"labels": {
				"cloud": "Other",
				"vendor": "Other"
			},
			"name": "mc1"
			},
			"spec": {
			"hubAcceptsClient": true,
			"leaseDurationSeconds": 60
			},
			"status": {
			"conditions": null,
			"version": {}
			}
		}
		},
		{
		"cells": [
			"mc2",
			null
		],
		"object": {
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind": "ManagedCluster",
			"metadata": {
			"uid": "18c9e13c-4488-4dcd-a5ac-1196093abbc0",
			"annotations": {
				"global-hub.open-cluster-management.io/managed-by": "hub1",
				"open-cluster-management/created-via": "other"
			},
			"creationTimestamp": null,
			"labels": {
				"cloud": "Other",
				"vendor": "Other"
			},
			"name": "mc2"
			},
			"spec": {
			"hubAcceptsClient": true,
			"leaseDurationSeconds": 60
			},
			"status": {
			"conditions": null,
			"version": {}
			}
		}
		}
	]
}
`
		w3 := httptest.NewRecorder()
		req3, err := http.NewRequest("GET", "/global-hub-api/v1/managedclusters", nil)
		Expect(err).ToNot(HaveOccurred())
		req3.Header.Set("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1")
		router.ServeHTTP(w3, req3)
		Expect(w3.Code).To(Equal(200))
		Expect(w3.Body.String()).Should(MatchJSON(mclTable))

		By("Check the managedcclusters can be listed with watch")
		w4 := CreateTestResponseRecorder()
		timeoutCtx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
		defer cancelFunc()
		req4, err := http.NewRequestWithContext(timeoutCtx, "GET",
			"/global-hub-api/v1/managedclusters?watch", nil)
		Expect(err).ToNot(HaveOccurred())
		go func() {
			router.ServeHTTP(w4, req4)
		}()
		// wait loop for client cancel the request
		for {
			select {
			case <-timeoutCtx.Done():
				w4.closeClient()
				return
			default:
				time.Sleep(4 * time.Second)
				Expect(w4.Code).To(Equal(200))
				// Expect(w4.Body.String()).Should(MatchJSON(fmt.Sprintf("[%s,%s]", mc1, mc2)))
			}
		}
	})

	It("Should be able to patch label(s) for managed cluster", func() {
		By("Patch managed cluster")
		w := httptest.NewRecorder()
		jsonPatchStr := []byte(`[
			{
				"op":    "add",
				"path":  "/metadata/labels/foo",
				"value": "bar"
			}
		]`)
		req, err := http.NewRequest("PATCH",
			"/global-hub-api/v1/managedcluster/2aa5547c-c172-47ed-b70b-db468c84d327",
			bytes.NewBuffer(jsonPatchStr))
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w, req)
		Expect(w.Code).To(Equal(200))

		By("Check the label for the managed cluster is patched")
		Eventually(func() bool {
			var labels map[string]string
			err = postgresSQL.GetConn().QueryRow(ctx,
				"SELECT labels from spec.managed_clusters_labels "+
					"WHERE id = $1;", "2aa5547c-c172-47ed-b70b-db468c84d327").Scan(&labels)
			return err == nil && labels["foo"] == "bar"
		}, 10*time.Second, 2*time.Second).Should(BeTrue())
	})

	It("Should be able to list policies", func() {
		plc1ID = uuid.New().String()
		pr1ID, pb1ID := uuid.New().String(), uuid.New().String()
		policy1, expectedPolicy1, placementrule1, placementbinding1 := `
{
	"apiVersion": "policy.open-cluster-management.io/v1",
	"kind": "Policy",
	"metadata": {
		"name": "policy-config-audit",
		"namespace": "default",
		"annotations": {
			"policy.open-cluster-management.io/standards": "NIST SP 800-53",
			"policy.open-cluster-management.io/categories": "AU Audit and Accountability",
			"policy.open-cluster-management.io/controls": "AU-3 Content of Audit Records"
		},
		"labels": {
			"env": "production",
			"foo": "bar"
		}
	},
	"spec": {
		"remediationAction": "inform",
		"disabled": false,
		"policy-templates": [
			{
				"objectDefinition": {
					"apiVersion": "policy.open-cluster-management.io/v1",
					"kind": "ConfigurationPolicy",
					"metadata": {
						"name": "policy-config-audit"
					},
					"spec": {
						"remediationAction": "inform",
						"severity": "low",
						"object-templates": [
							{
								"complianceType": "musthave",
								"objectDefinition": {
									"apiVersion": "config.openshift.io/v1",
									"kind": "APIServer",
									"metadata": {
										"name": "cluster"
									},
									"spec": {
										"audit": {
											"customRules": [
												{
													"group": "system:authenticated:oauth",
													"profile": "WriteRequestBodies"
												},
												{
													"group": "system:authenticated",
													"profile": "AllRequestBodies"
												}
											]
										},
										"profile": "Default"
									}
								}
							}
						]
					}
				}
			}
		]
	},
	"status": {}
}
`, `
{
	"apiVersion": "policy.open-cluster-management.io/v1",
	"kind": "Policy",
	"metadata": {
		"name": "policy-config-audit",
		"namespace": "default",
		"creationTimestamp": null,
		"annotations": {
			"policy.open-cluster-management.io/standards": "NIST SP 800-53",
			"policy.open-cluster-management.io/categories": "AU Audit and Accountability",
			"policy.open-cluster-management.io/controls": "AU-3 Content of Audit Records"
		},
		"labels": {
			"env": "production",
			"foo": "bar"
		}
	},
	"spec": {
		"remediationAction": "inform",
		"disabled": false,
		"policy-templates": [
			{
				"objectDefinition": {
					"apiVersion": "policy.open-cluster-management.io/v1",
					"kind": "ConfigurationPolicy",
					"metadata": {
						"name": "policy-config-audit"
					},
					"spec": {
						"remediationAction": "inform",
						"severity": "low",
						"object-templates": [
							{
								"complianceType": "musthave",
								"objectDefinition": {
									"apiVersion": "config.openshift.io/v1",
									"kind": "APIServer",
									"metadata": {
										"name": "cluster"
									},
									"spec": {
										"audit": {
											"customRules": [
												{
													"group": "system:authenticated:oauth",
													"profile": "WriteRequestBodies"
												},
												{
													"group": "system:authenticated",
													"profile": "AllRequestBodies"
												}
											]
										},
										"profile": "Default"
									}
								}
							}
						]
					}
				}
			}
		]
	},
	"status": {
		"placement": [
			{
				"placementBinding": "binding-config-audit",
				"placementRule": "placement-config-audit"
			}
		],
		"status": [
			{
				"compliant": "NonCompliant",
				"clustername": "mc1",
				"clusternamespace": "mc1"
			},
			{
				"compliant": "Compliant",
				"clustername": "mc2",
				"clusternamespace": "mc2"
			}
		],
		"summary": {
			"complianceClusterNumber": 1,
			"nonComplianceClusterNumber": 1
		},
		"compliant": "NonCompliant"
	}
}
`, `
{
    "apiVersion": "apps.open-cluster-management.io/v1",
    "kind": "PlacementRule",
    "metadata": {
        "name": "placement-config-audit",
		"namespace": "default"
    },
    "spec": {
        "clusterConditions": [
            {
                "status": "True",
                "type": "ManagedClusterConditionAvailable"
            }
        ],
        "clusterSelector": {
            "matchExpressions": [
                {
                    "key": "environment"
                }
            ]
        }
    },
    "operator": "In",
    "values": [
        "dev"
    ]
}
`, `
{
    "apiVersion": "policy.open-cluster-management.io/v1",
    "kind": "PlacementBinding",
    "metadata": {
        "name": "binding-config-audit",
		"namespace": "default"
    },
    "placementRef": {
        "name": "placement-config-audit",
        "kind": "PlacementRule",
        "apiGroup": "apps.open-cluster-management.io"
    },
    "subjects": [
        {
            "name": "policy-config-audit",
            "kind": "Policy",
            "apiGroup": "policy.open-cluster-management.io"
        }
    ]
}
`

		By("Check the empty policies can be listed without parameters")
		w0 := httptest.NewRecorder()
		req0, err := http.NewRequest("GET", "/global-hub-api/v1/policies", nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w0, req0)
		Expect(w0.Code).To(Equal(200))
		policyListFormatStr := `
		{
		"kind": "PolicyList",
		"apiVersion": "policy.open-cluster-management.io/v1",
		"items": []
		}`
		Expect(w0.Body.String()).Should(MatchJSON(policyListFormatStr))

		By("Insert testing policy")
		_, err = postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO spec.policies (id,payload) VALUES($1, $2);`, plc1ID, policy1)
		Expect(err).ToNot(HaveOccurred())

		By("Insert testing placementrule")
		_, err = postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO spec.placementrules (id,payload) VALUES($1, $2);`, pr1ID, placementrule1)
		Expect(err).ToNot(HaveOccurred())

		By("Insert testing placementbinding")
		_, err = postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO spec.placementbindings (id,payload) VALUES($1, $2);`, pb1ID, placementbinding1)
		Expect(err).ToNot(HaveOccurred())

		By("Insert testing compliances")
		_, err = postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO status.compliance (id,cluster_name,leaf_hub_name,error,compliance)
			VALUES($1,'mc1','hub1','none','non_compliant');`,
			plc1ID)
		Expect(err).ToNot(HaveOccurred())

		_, err = postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO status.compliance (id,cluster_name,leaf_hub_name,error,compliance)
			VALUES($1,'mc2','hub1','none','compliant');`,
			plc1ID)
		Expect(err).ToNot(HaveOccurred())

		By("Check the policies can be listed without parameters")
		w1 := httptest.NewRecorder()
		req1, err := http.NewRequest("GET", "/global-hub-api/v1/policies", nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w1, req1)
		Expect(w1.Code).To(Equal(200))
		policyListFormatStr = `
{
	"kind": "PolicyList",
	"apiVersion": "policy.open-cluster-management.io/v1",
	"items": [
		%s
		]
}`
		Expect(w1.Body.String()).Should(MatchJSON(
			fmt.Sprintf(policyListFormatStr, expectedPolicy1)))

		By("Check the policies can be listed with limit and labelSelector")
		w2 := httptest.NewRecorder()
		req2, err := http.NewRequest("GET",
			"/global-hub-api/v1/policies?"+
				"labelSelector=foo%3Dbar%2Cenv%21%3Ddev%2C%21testnokey%2Cfoo",
			nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w2, req2)
		Expect(w2.Code).To(Equal(200))
		Expect(w2.Body.String()).Should(MatchJSON(
			fmt.Sprintf(policyListFormatStr, expectedPolicy1)))

		By("Check the policies can be listed as table")
		plcTable := `
{
	"kind": "Table",
	"apiVersion": "meta.k8s.io/v1",
	"metadata": {},
	"columnDefinitions": [
		{
		"name": "Name",
		"type": "string",
		"format": "name",
		"description": "Name must be unique within a namespace. Is required when creating resources, although some resources may allow a client to request the generation of an appropriate name automatically. Name is primarily intended for creation idempotence and configuration definition. Cannot be updated. More info: http://kubernetes.io/docs/user-guide/identifiers#names",
		"priority": 0
		},
		{
		"name": "Age",
		"type": "date",
		"format": "",
		"description": "Custom resource definition column (in JSONPath format): .metadata.creationTimestamp",
		"priority": 0
		}
	],
	"rows": [
		{
		"cells": [
			"policy-config-audit",
			null
		],
		"object": {
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind": "Policy",
			"metadata": {
			"annotations": {
				"policy.open-cluster-management.io/categories": "AU Audit and Accountability",
				"policy.open-cluster-management.io/controls": "AU-3 Content of Audit Records",
				"policy.open-cluster-management.io/standards": "NIST SP 800-53"
			},
			"labels": {
				"env": "production",
				"foo": "bar"
			},
			"creationTimestamp": null,
			"name": "policy-config-audit",
			"namespace": "default"
			},
			"spec": {
			"disabled": false,
			"policy-templates": [
				{
				"objectDefinition": {
					"apiVersion": "policy.open-cluster-management.io/v1",
					"kind": "ConfigurationPolicy",
					"metadata": {
					"name": "policy-config-audit"
					},
					"spec": {
					"object-templates": [
						{
						"complianceType": "musthave",
						"objectDefinition": {
							"apiVersion": "config.openshift.io/v1",
							"kind": "APIServer",
							"metadata": {
							"name": "cluster"
							},
							"spec": {
							"audit": {
								"customRules": [
								{
									"group": "system:authenticated:oauth",
									"profile": "WriteRequestBodies"
								},
								{
									"group": "system:authenticated",
									"profile": "AllRequestBodies"
								}
								]
							},
							"profile": "Default"
							}
						}
						}
					],
					"remediationAction": "inform",
					"severity": "low"
					}
				}
				}
			],
			"remediationAction": "inform"
			},
			"status": {
			"compliant": "NonCompliant",
			"placement": [
				{
				"placementBinding": "binding-config-audit",
				"placementRule": "placement-config-audit"
				}
			],
			"status": [
				{
				"clustername": "mc1",
				"clusternamespace": "mc1",
				"compliant": "NonCompliant"
				},
				{
				"clustername": "mc2",
				"clusternamespace": "mc2",
				"compliant": "Compliant"
				}
			],
			"summary": {
				"complianceClusterNumber": 1,
				"nonComplianceClusterNumber": 1
			}
			}
		}
		}
	]
}
`
		w3 := httptest.NewRecorder()
		req3, err := http.NewRequest("GET", "/global-hub-api/v1/policies", nil)
		Expect(err).ToNot(HaveOccurred())
		req3.Header.Set("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1")
		router.ServeHTTP(w3, req3)
		Expect(w3.Code).To(Equal(200))
		Expect(w3.Body.String()).Should(MatchJSON(plcTable))

		By("Check the policies can be listed with watch")
		w4 := CreateTestResponseRecorder()
		timeoutCtx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
		defer cancelFunc()
		req4, err := http.NewRequestWithContext(timeoutCtx, "GET",
			"/global-hub-api/v1/policies?watch", nil)
		Expect(err).ToNot(HaveOccurred())
		go func() {
			router.ServeHTTP(w4, req4)
		}()
		// wait loop for client cancel the request
		for {
			select {
			case <-timeoutCtx.Done():
				w4.closeClient()
				return
			default:
				time.Sleep(4 * time.Second)
				Expect(w4.Code).To(Equal(200))
				// Expect(w4.Body.String()).Should(MatchJSON(fmt.Sprintf("[%s]", expectedPolicy1)))
			}
		}
	})

	It("Should be able to get policy status", func() {
		expectedPolicyStatus1 := `
{
	"apiVersion": "policy.open-cluster-management.io/v1",
	"kind": "Policy",
	"metadata": {
		"name": "policy-config-audit",
		"namespace": "default",
		"creationTimestamp": null,
		"annotations": {
			"policy.open-cluster-management.io/categories": "AU Audit and Accountability",
			"policy.open-cluster-management.io/controls": "AU-3 Content of Audit Records",
			"policy.open-cluster-management.io/standards": "NIST SP 800-53"
		},
		"labels": {
			"env": "production",
			"foo": "bar"
		}
	},
	"status": {
		"placement": [
			{
				"placementBinding": "binding-config-audit",
				"placementRule": "placement-config-audit"
			}
		],
		"status": [
			{
				"compliant": "NonCompliant",
				"clustername": "mc1",
				"clusternamespace": "mc1"
			},
			{
				"compliant": "Compliant",
				"clustername": "mc2",
				"clusternamespace": "mc2"
			}
		],
		"summary": {
			"complianceClusterNumber": 1,
			"nonComplianceClusterNumber": 1
		},
		"compliant": "NonCompliant"
	}
}
`

		By("Check the policy status can be retrieved with policy ID")
		w1 := httptest.NewRecorder()
		req1, err := http.NewRequest("GET", fmt.Sprintf(
			"/global-hub-api/v1/policy/%s/status", plc1ID), nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w1, req1)
		Expect(w1.Code).To(Equal(200))
		Expect(w1.Body.String()).Should(MatchJSON(expectedPolicyStatus1))

		By("Check the policy status can be retrieved with policy ID as table")
		plcTable := `
{
	"kind": "Table",
	"apiVersion": "meta.k8s.io/v1",
	"metadata": {},
	"columnDefinitions": [
		{
		"name": "Name",
		"type": "string",
		"format": "name",
		"description": "Name must be unique within a namespace. Is required when creating resources, although some resources may allow a client to request the generation of an appropriate name automatically. Name is primarily intended for creation idempotence and configuration definition. Cannot be updated. More info: http://kubernetes.io/docs/user-guide/identifiers#names",
		"priority": 0
		},
		{
		"name": "Age",
		"type": "date",
		"format": "",
		"description": "Custom resource definition column (in JSONPath format): .metadata.creationTimestamp",
		"priority": 0
		}
	],
	"rows": [
		{
		"cells": [
			"policy-config-audit",
			null
		],
		"object": {
			"kind": "Policy",
			"apiVersion": "policy.open-cluster-management.io/v1",
			"metadata": {
			"name": "policy-config-audit",
			"namespace": "default",
			"creationTimestamp": null,
			"annotations": {
				"policy.open-cluster-management.io/categories": "AU Audit and Accountability",
				"policy.open-cluster-management.io/controls": "AU-3 Content of Audit Records",
				"policy.open-cluster-management.io/standards": "NIST SP 800-53"
			},
			"labels": {
				"env": "production",
				"foo": "bar"
			}
			},
			"status": {
			"placement": [
				{
				"placementBinding": "binding-config-audit",
				"placementRule": "placement-config-audit"
				}
			],
			"status": [
				{
				"compliant": "NonCompliant",
				"clustername": "mc1",
				"clusternamespace": "mc1"
				},
				{
				"compliant": "Compliant",
				"clustername": "mc2",
				"clusternamespace": "mc2"
				}
			],
			"compliant": "NonCompliant",
			"summary": {
				"complianceClusterNumber": 1,
				"nonComplianceClusterNumber": 1
			}
			}
		}
		}
	]
}
`
		w2 := httptest.NewRecorder()
		req2, err := http.NewRequest("GET", fmt.Sprintf(
			"/global-hub-api/v1/policy/%s/status", plc1ID), nil)
		Expect(err).ToNot(HaveOccurred())
		req2.Header.Set("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1")
		router.ServeHTTP(w2, req2)
		Expect(w2.Code).To(Equal(200))
		Expect(w2.Body.String()).Should(MatchJSON(plcTable))

		By("Check the policy status can be retrieved with watch")
		w3 := CreateTestResponseRecorder()
		timeoutCtx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
		defer cancelFunc()
		req3, err := http.NewRequestWithContext(timeoutCtx, "GET",
			fmt.Sprintf("/global-hub-api/v1/policy/%s/status?watch", plc1ID), nil)
		Expect(err).ToNot(HaveOccurred())
		go func() {
			router.ServeHTTP(w3, req3)
		}()
		// wait loop for client cancel the request
		for {
			select {
			case <-timeoutCtx.Done():
				w3.closeClient()
				return
			default:
				time.Sleep(4 * time.Second)
				Expect(w3.Code).To(Equal(200))
				// Expect(w3.Body.String()).Should(MatchJSON(fmt.Sprintf("%s", expectedPolicyStatus1)))
			}
		}
	})

	It("Should be able to list subscriptions", func() {
		sub1ID, sub2ID = uuid.New().String(), uuid.New().String()
		subscription1, subscription2 := `{
	"apiVersion": "apps.open-cluster-management.io/v1",
	"kind": "Subscription",
	"metadata": {
	  "annotations": {
		"apps.open-cluster-management.io/git-branch": "main",
		"apps.open-cluster-management.io/git-path": "bar",
		"apps.open-cluster-management.io/reconcile-option": "merge"
	  },
	  "labels": {
		"app": "bar",
		"app.kubernetes.io/part-of": "bar",
		"apps.open-cluster-management.io/reconcile-rate": "medium"
	  },
	  "name": "bar-appsub",
	  "namespace": "bar",
	  "creationTimestamp": null
	},
	"spec": {
	  "channel": "git-application-samples-ns/git-application-samples",
	  "placement": {
		"placementRef": {
		  "kind": "PlacementRule",
		  "name": "bar-placement"
		}
	  }
	},
	"status": {
      "lastUpdateTime": null,
	  "ansiblejobs": {}
    }
}`, `{
	"apiVersion": "apps.open-cluster-management.io/v1",
	"kind": "Subscription",
	"metadata": {
	  "annotations": {
		"apps.open-cluster-management.io/git-branch": "main",
		"apps.open-cluster-management.io/git-path": "foo",
		"apps.open-cluster-management.io/reconcile-option": "merge"
	  },
	  "labels": {
		"app": "foo",
		"app.kubernetes.io/part-of": "foo",
		"apps.open-cluster-management.io/reconcile-rate": "medium"
	  },
	  "name": "foo-appsub",
	  "namespace": "foo",
	  "creationTimestamp": null
	},
	"spec": {
	  "channel": "git-application-samples-ns/git-application-samples",
	  "placement": {
		"placementRef": {
		  "kind": "PlacementRule",
		  "name": "foo-placement"
		}
	  }
	},
	"status": {
	  "lastUpdateTime": null,
	  "ansiblejobs": {}
	}
}`

		By("Check the empty subscriptions can be listed without parameters")
		w0 := httptest.NewRecorder()
		req0, err := http.NewRequest("GET", "/global-hub-api/v1/subscriptions", nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w0, req0)
		Expect(w0.Code).To(Equal(200))
		subscriptionListFormatStr := `
{
	"kind": "SubscriptionList",
	"apiVersion": "apps.open-cluster-management.io/v1",
	"metadata": {},
	"items": []
}`
		Expect(w0.Body.String()).Should(MatchJSON(subscriptionListFormatStr))

		By("Insert testing subscriptions")
		_, err = postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO spec.subscriptions (id,payload) VALUES($1, $2);`, sub1ID, subscription1)
		Expect(err).ToNot(HaveOccurred())
		_, err = postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO spec.subscriptions (id,payload) VALUES($1, $2);`, sub2ID, subscription2)
		Expect(err).ToNot(HaveOccurred())

		By("Check the subscriptions can be listed without parameters")
		w1 := httptest.NewRecorder()
		req1, err := http.NewRequest("GET", "/global-hub-api/v1/subscriptions", nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w1, req1)
		Expect(w1.Code).To(Equal(200))
		subscriptionListFormatStr = `
{
	"kind": "SubscriptionList",
	"apiVersion": "apps.open-cluster-management.io/v1",
	"metadata": {},
	"items": [
		%s,
		%s
		]
}`
		Expect(w1.Body.String()).Should(MatchJSON(
			fmt.Sprintf(subscriptionListFormatStr, subscription1, subscription2)))

		By("Check the subscriptions can be listed with limit and labelSelector")
		w2 := httptest.NewRecorder()
		req2, err := http.NewRequest("GET",
			"/global-hub-api/v1/subscriptions?"+
				"labelSelector=app%3Dfoo%2Cenv%21%3Ddev%2C%21testnokey",
			nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w2, req2)
		Expect(w2.Code).To(Equal(200))
		subscriptionListFormatStr = `
		{
			"kind": "SubscriptionList",
			"apiVersion": "apps.open-cluster-management.io/v1",
			"metadata": {},
			"items": [
				%s
			]
		}`
		Expect(w2.Body.String()).Should(MatchJSON(
			fmt.Sprintf(subscriptionListFormatStr, subscription2)))

		By("Check the subscriptions can be listed as table")
		subscriptionTable := `{
			"kind": "Table",
			"apiVersion": "meta.k8s.io/v1",
			"metadata": {},
			"columnDefinitions": [
			  {
				"name": "Name",
				"type": "string",
				"format": "name",
				"description": "Name must be unique within a namespace. Is required when creating resources, although some resources may allow a client to request the generation of an appropriate name automatically. Name is primarily intended for creation idempotence and configuration definition. Cannot be updated. More info: http://kubernetes.io/docs/user-guide/identifiers#names",
				"priority": 0
			  },
			  {
				"name": "Age",
				"type": "date",
				"format": "",
				"description": "Custom resource definition column (in JSONPath format): .metadata.creationTimestamp",
				"priority": 0
			  }
			],
			"rows": [
			  {
				"cells": [
				  "bar-appsub",
				  null
				],
				"object": {
				  "apiVersion": "apps.open-cluster-management.io/v1",
				  "kind": "Subscription",
				  "metadata": {
					"annotations": {
					  "apps.open-cluster-management.io/git-branch": "main",
					  "apps.open-cluster-management.io/git-path": "bar",
					  "apps.open-cluster-management.io/reconcile-option": "merge"
					},
					"creationTimestamp": null,
					"labels": {
					  "app": "bar",
					  "app.kubernetes.io/part-of": "bar",
					  "apps.open-cluster-management.io/reconcile-rate": "medium"
					},
					"name": "bar-appsub",
					"namespace": "bar"
				  },
				  "spec": {
					"channel": "git-application-samples-ns/git-application-samples",
					"placement": {
					  "placementRef": {
						"kind": "PlacementRule",
						"name": "bar-placement"
					  }
					}
				  },
				  "status": {
					"ansiblejobs": {},
					"lastUpdateTime": null
				  }
				}
			  },
			  {
				"cells": [
				  "foo-appsub",
				  null
				],
				"object": {
				  "apiVersion": "apps.open-cluster-management.io/v1",
				  "kind": "Subscription",
				  "metadata": {
					"annotations": {
					  "apps.open-cluster-management.io/git-branch": "main",
					  "apps.open-cluster-management.io/git-path": "foo",
					  "apps.open-cluster-management.io/reconcile-option": "merge"
					},
					"creationTimestamp": null,
					"labels": {
					  "app": "foo",
					  "app.kubernetes.io/part-of": "foo",
					  "apps.open-cluster-management.io/reconcile-rate": "medium"
					},
					"name": "foo-appsub",
					"namespace": "foo"
				  },
				  "spec": {
					"channel": "git-application-samples-ns/git-application-samples",
					"placement": {
					  "placementRef": {
						"kind": "PlacementRule",
						"name": "foo-placement"
					  }
					}
				  },
				  "status": {
					"ansiblejobs": {},
					"lastUpdateTime": null
				  }
				}
			  }
			]
		  }`
		w3 := httptest.NewRecorder()
		req3, err := http.NewRequest("GET", "/global-hub-api/v1/subscriptions", nil)
		Expect(err).ToNot(HaveOccurred())
		req3.Header.Set("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1")
		router.ServeHTTP(w3, req3)
		Expect(w3.Code).To(Equal(200))
		Expect(w3.Body.String()).Should(MatchJSON(subscriptionTable))

		By("Check the subscriptions can be listed with watch")
		w4 := CreateTestResponseRecorder()
		timeoutCtx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
		defer cancelFunc()
		req4, err := http.NewRequestWithContext(timeoutCtx, "GET",
			"/global-hub-api/v1/subscriptions?watch", nil)
		Expect(err).ToNot(HaveOccurred())
		go func() {
			router.ServeHTTP(w4, req4)
		}()
		// wait loop for client cancel the request
		for {
			select {
			case <-timeoutCtx.Done():
				w4.closeClient()
				return
			default:
				time.Sleep(4 * time.Second)
				Expect(w4.Code).To(Equal(200))
				// Expect(w4.Body.String()).Should(MatchJSON(fmt.Sprintf("[%s]", subscription1)))
			}
		}
	})

	It("Should be able to get subscriptionreport", func() {
		subReportHub1ID, subReportHub2ID := uuid.New().String(), uuid.New().String()
		leafhub1, leafhub2 := "hub1", "hub2"
		subReportHub1, subReportHub2 := `{
	"kind": "SubscriptionReport",
	"apiVersion": "apps.open-cluster-management.io/v1alpha1",
	"metadata": {
		"name": "foo-appsub",
		"labels": {
			"apps.open-cluster-management.io/hosting-subscription": "foo.foo-appsub"
		},
		"namespace": "foo",
		"resourceVersion": "2633120",
		"creationTimestamp": "2022-10-13T05:58:33Z"
	},
	"reportType": "Application",
	"results": [
		{
			"result": "deployed",
			"source": "mc1",
			"timestamp": {
				"nanos": 0,
				"seconds": 0
			}
		}
	],
	"summary": {
		"failed": "0",
		"clusters": "1",
		"deployed": "1",
		"inProgress": "0",
		"propagationFailed": "0"
	},
	"resources": [
		{
			"kind": "Route",
			"name": "foo-app-route",
			"namespace": "foo",
			"apiVersion": "route.openshift.io/v1"
		},
		{
			"kind": "Service",
			"name": "foo-app-svc",
			"namespace": "foo",
			"apiVersion": "v1"
		},
		{
			"kind": "Deployment",
			"name": "foo-app-deploy",
			"namespace": "foo",
			"apiVersion": "apps/v1"
		}
	]
}`, `{
	"kind": "SubscriptionReport",
	"apiVersion": "apps.open-cluster-management.io/v1alpha1",
	"metadata": {
		"name": "foo-appsub",
		"labels": {
			"apps.open-cluster-management.io/hosting-subscription": "foo.foo-appsub"
		},
		"namespace": "foo",
		"resourceVersion": "2633112",
		"creationTimestamp": "2022-10-13T05:58:31Z"
	},
	"reportType": "Application",
	"results": [
		{
			"result": "deployed",
			"source": "mc2",
			"timestamp": {
				"nanos": 0,
				"seconds": 0
			}
		}
	],
	"summary": {
		"failed": "0",
		"clusters": "1",
		"deployed": "1",
		"inProgress": "0",
		"propagationFailed": "0"
	},
	"resources": [
		{
			"kind": "Route",
			"name": "foo-app-route",
			"namespace": "foo",
			"apiVersion": "route.openshift.io/v1"
		},
		{
			"kind": "Service",
			"name": "foo-app-svc",
			"namespace": "foo",
			"apiVersion": "v1"
		},
		{
			"kind": "Deployment",
			"name": "foo-app-deploy",
			"namespace": "foo",
			"apiVersion": "apps/v1"
		}
	]
}`

		By("Insert testing subscription report for leaf hub")
		_, err := postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO status.subscription_reports (id,leaf_hub_name,payload) VALUES($1, $2, $3);`,
			subReportHub1ID, leafhub1, subReportHub1)
		Expect(err).ToNot(HaveOccurred())
		_, err = postgresSQL.GetConn().Exec(ctx,
			`INSERT INTO status.subscription_reports (id,leaf_hub_name,payload) VALUES($1, $2, $3);`,
			subReportHub2ID, leafhub2, subReportHub2)
		Expect(err).ToNot(HaveOccurred())

		By("Check the subscriptionreport can be retrieved")
		w1 := httptest.NewRecorder()
		req1, err := http.NewRequest("GET", fmt.Sprintf(
			"/global-hub-api/v1/subscriptionreport/%s", sub2ID), nil)
		Expect(err).ToNot(HaveOccurred())
		router.ServeHTTP(w1, req1)
		Expect(w1.Code).To(Equal(200))
		subscriptionReportStr := `{
			"kind": "SubscriptionReport",
			"apiVersion": "apps.open-cluster-management.io/v1alpha1",
			"metadata": {
			  "name": "foo-appsub",
			  "namespace": "foo",
			  "resourceVersion": "2633120",
			  "creationTimestamp": "2022-10-13T05:58:33Z",
			  "labels": {
				"apps.open-cluster-management.io/hosting-subscription": "foo.foo-appsub"
			  }
			},
			"reportType": "Application",
			"summary": {
			  "deployed": "2",
			  "inProgress": "0",
			  "failed": "0",
			  "propagationFailed": "0",
			  "clusters": "2"
			},
			"results": [
			  {
				"source": "mc1",
				"timestamp": {
				  "seconds": 0,
				  "nanos": 0
				},
				"result": "deployed"
			  },
			  {
				"source": "mc2",
				"timestamp": {
				  "seconds": 0,
				  "nanos": 0
				},
				"result": "deployed"
			  }
			],
			"resources": [
			  {
				"kind": "Route",
				"namespace": "foo",
				"name": "foo-app-route",
				"apiVersion": "route.openshift.io/v1"
			  },
			  {
				"kind": "Service",
				"namespace": "foo",
				"name": "foo-app-svc",
				"apiVersion": "v1"
			  },
			  {
				"kind": "Deployment",
				"namespace": "foo",
				"name": "foo-app-deploy",
				"apiVersion": "apps/v1"
			  }
			]
		  }`
		Expect(w1.Body.String()).Should(MatchJSON(subscriptionReportStr))
	})

	AfterAll(func() {
		cancel()
		postgresSQL.Stop()
	})
})
