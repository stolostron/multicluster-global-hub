do
$$
declare
    managed_cluster_template text := '{ "apiVersion": "cluster.open-cluster-management.io/v1", "kind": "ManagedCluster", "metadata": { "annotations": { "open-cluster-management/created-via": "other" }, "labels": { "cloud": "Amazon", "cluster.open-cluster-management.io/clusterset": "default", "feature.open-cluster-management.io/addon-application-manager": "available", "feature.open-cluster-management.io/addon-cert-policy-controller": "available", "feature.open-cluster-management.io/addon-cluster-proxy": "available", "feature.open-cluster-management.io/addon-config-policy-controller": "available", "feature.open-cluster-management.io/addon-governance-policy-framework": "available", "feature.open-cluster-management.io/addon-iam-policy-controller": "available", "feature.open-cluster-management.io/addon-work-manager": "available", "name": "cluster1", "clusterID": "00000000-0000-0000-0000-000000000000", "openshiftVersion": "4.10.36", "openshiftVersion-major": "4", "openshiftVersion-major-minor": "4.10", "vendor": "OpenShift" }, "name": "cluster1" }, "spec": { "hubAcceptsClient": true, "leaseDurationSeconds": 60, "managedClusterClientConfigs": [ { "url": "https://cluster1.example.com:6443" } ] }, "status": { "version": { "kubernetes": "v1.25.8+37a9a08" }, "capacity": { "cpu": "16", "pods": "250", "memory": "64551700Ki", "core_worker": "16", "hugepages-1Gi": "0", "hugepages-2Mi": "0", "socket_worker": "0", "ephemeral-storage": "125293548Ki", "attachable-volumes-aws-ebs": "39" }, "conditions": [ { "type": "ManagedClusterImportSucceeded", "reason": "ManagedClusterImported", "status": "True", "message": "Import succeeded", "lastTransitionTime": "2023-10-17T00:05:24Z" }, { "type": "HubAcceptedManagedCluster", "reason": "HubClusterAdminAccepted", "status": "True", "message": "Accepted by hub cluster admin", "lastTransitionTime": "2023-10-17T00:05:11Z" }, { "type": "ManagedClusterJoined", "reason": "ManagedClusterJoined", "status": "True", "message": "Managed cluster joined", "lastTransitionTime": "2023-10-17T00:05:21Z" }, { "type": "ManagedClusterConditionAvailable", "reason": "ManagedClusterAvailable", "status": "True", "message": "Managed cluster is available", "lastTransitionTime": "2023-10-17T00:05:21Z" } ], "allocatable": { "cpu": "15500m", "pods": "250", "memory": "63400724Ki", "hugepages-1Gi": "0", "hugepages-2Mi": "0", "ephemeral-storage": "114396791822", "attachable-volumes-aws-ebs": "39" }, "clusterClaims": [ { "name": "id.k8s.io", "value": "00000000-0000-0000-0000-000000000000" }, { "name": "kubeversion.open-cluster-management.io", "value": "v1.25.8+37a9a08" }, { "name": "platform.open-cluster-management.io", "value": "AWS" }, { "name": "product.open-cluster-management.io", "value": "OpenShift" }, { "name": "above.threshold.hostedclustercount.hypershift.openshift.io", "value": "false" }, { "name": "consoleurl.cluster.open-cluster-management.io", "value": "https://console-openshift-console.apps.com" }, { "name": "controlplanetopology.openshift.io", "value": "SingleReplica" }, { "name": "full.hostedclustercount.hypershift.openshift.io", "value": "false" }, { "name": "hostingcluster.hypershift.openshift.io", "value": "true" }, { "name": "hub.open-cluster-management.io", "value": "InstalledByUser" }, { "name": "id.openshift.io", "value": "00000000-0000-0000-0000-000000000000" }, { "name": "infrastructure.openshift.io", "value": "{\"infraName\":\"obs-hub-of-hubs-aws-4-pwlt8\"}" }, { "name": "oauthredirecturis.openshift.io", "value": "https://oauth-openshift.apps" }, { "name": "region.open-cluster-management.io", "value": "us-east-1" }, { "name": "version.open-cluster-management.io", "value": "2.9.0" }, { "name": "version.openshift.io", "value": "4.12.18" }, { "name": "zero.hostedclustercount.hypershift.openshift.io", "value": "true" } ] } }';
    policy_template text := '{"apiVersion":"policy.open-cluster-management.io/v1","kind":"Policy","metadata":{"uid":"d54ee051-0cbf-4a6c-ba8c-a5ad99acb295","name":"policy-config-audit","namespace":"default","annotations":{"policy.open-cluster-management.io/standards":"policy_standard_template","policy.open-cluster-management.io/categories":"policy_category_template","policy.open-cluster-management.io/controls":"policy_control_template"}},"spec":{"remediationAction":"inform","disabled":false,"policy-templates":[{"objectDefinition":{"apiVersion":"policy.open-cluster-management.io/v1","kind":"ConfigurationPolicy","metadata":{"name":"policy-config-audit"},"spec":{"remediationAction":"inform","severity":"policy_severity_template","object-templates":[{"complianceType":"musthave","objectDefinition":{"apiVersion":"config.openshift.io/v1","kind":"APIServer","metadata":{"name":"cluster"},"spec":{"audit":{"customRules":[{"group":"system:authenticated:oauth","profile":"WriteRequestBodies"},{"group":"system:authenticated","profile":"AllRequestBodies"}]},"profile":"Default"}}}]}}}]}}';
    all_event_policy text[] := '{"Policy default.policy-config-audit status was updated to Compliant in cluster namespace cluster-config","Policy default.policy-config-audit status was updated to NonCompliant in cluster namespace cluster-config"}';
    event_policy_random_index int;
    all_event_root_policy text[] := '{"Policy default/policy-config-audit was propagated for cluster cluster-config/cluster-config","Policy default/policy-config-audit was updated for cluster cluster-config/cluster-config"}';
    all_policy_standards text[] := '{"NIST SP 800-53","NIST-CSF"}';
    policy_standard_random_index int;
    all_policy_categories text[] := '{"AC Access Control","AU Audit and Accountability","PR.IP Information Protection Processes and Procedures","CM Configuration Management"}';
    policy_category_random_index int;
    all_policy_controls text[] := '{"AC-3 Access Enforcement","AC-2 Account Management","AC-6 Least Privilege","AU-3 Content of Audit Records","PR.IP-1 Baseline Configuration","CM-2 Baseline Configuration"}';
    policy_control_random_index int;
    all_policy_severities text[] := '{"low","high"}';
    policy_severity_random_index int;
    all_compliances local_status.compliance_type[] := '{"compliant","unknown","pending","non_compliant"}';
    compliance_random_index int;
    managed_cluster text;
    policy text;
    managed_cluster_name text;
    policy_name text;
    managed_cluster_id uuid;
    policy_id uuid;
    managed_cluster_ids uuid[];
    policy_ids uuid[];
    managed_cluster_index_insert int = 0;
    managed_cluster_index int = 0;
    policy_index int = 0;
    hub_name text;
    event_name text;
    event_message text;
    reason text;
    source text;
    count int = 0;
begin
    CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
    for i in 1..3 loop
        hub_name = format('leaf-hub-%s', i);
        raise notice 'hub name %', hub_name;

        policy_ids = '{}';
        for i in 1..30 loop
            policy_ids  = array_append(policy_ids, uuid_generate_v4());
        end loop;
        raise notice 'policy_ids %', policy_ids;

        managed_cluster_ids = '{}';
        for i in 1..233 loop
            managed_cluster_ids  = array_append(managed_cluster_ids, uuid_generate_v4());
        end loop;
        raise notice 'managed_cluster_ids %', managed_cluster_ids;

        foreach managed_cluster_id in array managed_cluster_ids loop
            managed_cluster_index_insert = managed_cluster_index_insert + 1;
            managed_cluster_name = format('cluster-%s', managed_cluster_index_insert);
            raise notice 'managedcluster name %', managed_cluster_name;
            managed_cluster = REPLACE(managed_cluster_template, 'cluster1', managed_cluster_name);
            managed_cluster = REPLACE(managed_cluster, '00000000-0000-0000-0000-000000000000', text(managed_cluster_id));
            insert into status.managed_clusters (leaf_hub_name,cluster_id,payload,error) values (hub_name,managed_cluster_id,jsonb(managed_cluster),'none');
        end loop;

        foreach policy_id in array policy_ids loop
            policy_index = policy_index + 1;
            policy_name = format('policy-%s', policy_index);
            raise notice 'policy name %', policy_name;
            policy = REPLACE(policy_template, 'policy-config-audit', policy_name);
            policy = REPLACE(policy, 'd54ee051-0cbf-4a6c-ba8c-a5ad99acb295', text(policy_id));
            SELECT floor(random() * 2 + 1)::int into policy_standard_random_index;
            policy = REPLACE(policy, 'policy_standard_template', all_policy_standards[policy_standard_random_index]);
            SELECT floor(random() * 4 + 1)::int into policy_category_random_index;
            policy = REPLACE(policy, 'policy_category_template', all_policy_categories[policy_category_random_index]);
            SELECT floor(random() * 6 + 1)::int into policy_control_random_index;
            policy = REPLACE(policy, 'policy_control_template', all_policy_controls[policy_control_random_index]);
            SELECT floor(random() * 2 + 1)::int into policy_severity_random_index;
            policy = REPLACE(policy, 'policy_severity_template', all_policy_severities[policy_severity_random_index]);
            insert into local_spec.policies (leaf_hub_name,policy_id,payload) values (hub_name,policy_id,jsonb(policy));
        end loop;

        foreach managed_cluster_id in array managed_cluster_ids loop
            managed_cluster_index = managed_cluster_index + 1;
            managed_cluster_name = format('cluster-%s', managed_cluster_index);
            raise notice 'managedcluster name %', managed_cluster_name;
            SELECT floor(random() * 2 + 1)::int into compliance_random_index;
            foreach policy_id in array policy_ids loop
                insert into local_status.compliance (policy_id,cluster_id,cluster_name,leaf_hub_name,compliance,error) VALUES (policy_id,managed_cluster_id,managed_cluster_name,hub_name,all_compliances[compliance_random_index],'none');
            end loop;
        end loop;

        managed_cluster_index = managed_cluster_index - 233;
        foreach managed_cluster_id in array managed_cluster_ids loop
            managed_cluster_index = managed_cluster_index + 1;
            managed_cluster_name = format('cluster-%s', managed_cluster_index);
            raise notice 'event - managedcluster name %', managed_cluster_name;

            policy_index = policy_index - 30;
            foreach policy_id in array policy_ids loop
                policy_index = policy_index + 1;
                policy_name = format('policy-%s', policy_index);
                raise notice 'event - policy name %', policy_name;

                count = count + 1;

                SELECT floor(random() * 2 + 1)::int into event_policy_random_index;
                event_message = all_event_policy[event_policy_random_index];
                event_message = REPLACE(event_message, 'policy-config-audit', policy_name);
                event_message = REPLACE(event_message, 'cluster-config', managed_cluster_name);
                event_name = format('PolicyEvent-%s', managed_cluster_index + 1);
                reason = 'PolicyStatusSync';
                source = '{"component": "policy-status-sync"}';
                insert into event.local_policies (event_name,policy_id,cluster_id,leaf_hub_name,message,reason,count,source,compliance) values (event_name,policy_id,managed_cluster_id,hub_name,event_message,reason,count,jsonb(source),all_compliances[event_policy_random_index]);

                event_message = all_event_root_policy[event_policy_random_index];
                event_message = REPLACE(event_message, 'policy-config-audit', policy_name);
                event_message = REPLACE(event_message, 'cluster-config', managed_cluster_name);
                event_name = format('RootPolicyEvent-%s', managed_cluster_index + 1);
                reason = 'PolicyPropagation';
                source = '{"component": "PolicyPropagation"}';
                insert into event.local_root_policies (event_name,policy_id,leaf_hub_name,message,reason,count,source,compliance) values (event_name,policy_id,hub_name,event_message,reason,count,jsonb(source),all_compliances[event_policy_random_index]);
            end loop;
        end loop;
    end loop;
end;
$$;
