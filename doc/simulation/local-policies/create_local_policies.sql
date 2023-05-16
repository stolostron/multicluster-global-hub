do
$$
declare
    managed_cluster_template text := '{"apiVersion":"cluster.open-cluster-management.io/v1","kind":"ManagedCluster","metadata":{"annotations":{"open-cluster-management/created-via":"other"},"labels":{"cloud":"Amazon","cluster.open-cluster-management.io/clusterset":"default","feature.open-cluster-management.io/addon-application-manager":"available","feature.open-cluster-management.io/addon-cert-policy-controller":"available","feature.open-cluster-management.io/addon-cluster-proxy":"available","feature.open-cluster-management.io/addon-config-policy-controller":"available","feature.open-cluster-management.io/addon-governance-policy-framework":"available","feature.open-cluster-management.io/addon-iam-policy-controller":"available","feature.open-cluster-management.io/addon-work-manager":"available","name":"cluster1","openshiftVersion":"4.10.36","openshiftVersion-major":"4","openshiftVersion-major-minor":"4.10","vendor":"OpenShift"},"name":"cluster1"},"spec":{"hubAcceptsClient":true,"leaseDurationSeconds":60,"managedClusterClientConfigs":[{"caBundle":"xxxxxx","url":"https://cluster1.example.com:6443"}]}}';
    policy_template text := '{"apiVersion":"policy.open-cluster-management.io/v1","kind":"Policy","metadata":{"uid":"d54ee051-0cbf-4a6c-ba8c-a5ad99acb295","name":"policy-config-audit","annotations":{"policy.open-cluster-management.io/standards":"NIST SP 800-53","policy.open-cluster-management.io/categories":"AU Audit and Accountability","policy.open-cluster-management.io/controls":"AU-3 Content of Audit Records"}},"spec":{"remediationAction":"inform","disabled":false,"policy-templates":[{"objectDefinition":{"apiVersion":"policy.open-cluster-management.io/v1","kind":"ConfigurationPolicy","metadata":{"name":"policy-config-audit"},"spec":{"remediationAction":"inform","severity":"low","object-templates":[{"complianceType":"musthave","objectDefinition":{"apiVersion":"config.openshift.io/v1","kind":"APIServer","metadata":{"name":"cluster"},"spec":{"audit":{"customRules":[{"group":"system:authenticated:oauth","profile":"WriteRequestBodies"},{"group":"system:authenticated","profile":"AllRequestBodies"}]},"profile":"Default"}}}]}}}]}}';
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
            insert into status.managed_clusters (leaf_hub_name,cluster_id,payload,error) values (hub_name,managed_cluster_id,jsonb(managed_cluster),'none');
        end loop;

        foreach policy_id in array policy_ids loop
            policy_index = policy_index + 1;
            policy_name = format('policy-%s', policy_index);
            raise notice 'policy name %', policy_name;
            policy = REPLACE(policy_template, 'policy-config-audit', policy_name);
            policy = REPLACE(policy, 'd54ee051-0cbf-4a6c-ba8c-a5ad99acb295', text(policy_id));
            insert into local_spec.policies (leaf_hub_name,payload) values (hub_name,jsonb(policy));
        end loop;

        foreach managed_cluster_id in array managed_cluster_ids loop
            managed_cluster_index = managed_cluster_index + 1;
            managed_cluster_name = format('cluster-%s', managed_cluster_index);
            raise notice 'managedcluster name %', managed_cluster_name;
            foreach policy_id in array policy_ids loop
                insert into local_status.compliance (id,cluster_id,cluster_name,leaf_hub_name,compliance,error) VALUES (policy_id,managed_cluster_id,managed_cluster_name,hub_name,'compliant','none');
            end loop;
        end loop;

    end loop;
end;
$$;
