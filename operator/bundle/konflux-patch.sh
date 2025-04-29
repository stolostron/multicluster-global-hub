#! /bin/bash

set -e

export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE=quay.io/redhat-user-workloads/acm-multicluster-glo-tenant/multicluster-global-hub-agent-globalhub-1-5@sha256:40a1f204121c781d170090733bb49bae744593ffab51dc7d19c849b6ca3c34d3
export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE=quay.io/redhat-user-workloads/acm-multicluster-glo-tenant/multicluster-global-hub-manager-globalhub-1-5@sha256:f2d856ad41c8ce2aeb419905fb4d5cd0e7e6af2724d5675c8e9e28418630899e
export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE=quay.io/redhat-user-workloads/acm-multicluster-glo-tenant/multicluster-global-hub-operator-globalhub-1-5@sha256:07c5265ee903493a4edfb883301b0c759c14fc14d74373bf3b2ff3479688633a
export MULTICLUSTER_GLOBAL_HUB_GRAFANA_IMAGE=quay.io/redhat-user-workloads/acm-multicluster-glo-tenant/glo-grafana-globalhub-1-5@sha256:8e8e311109af8f47d1afa068e4a42d6d8f6fe5d8a58515f8c65821b2e62d6723
export MULTICLUSTER_GLOBAL_HUB_POSTGRES_EXPORTER_IMAGE=quay.io/redhat-user-workloads/acm-multicluster-glo-tenant/postgres-exporter-globalhub-1-5@sha256:02ba9ce7dc27b2e404eb309b40726bf0e1513367a14f9ec0a0256753c74a072d
export MULTICLUSTER_GLOBAL_HUB_KESSEL_INVENTORY_API_IMAGE=quay.io/redhat-services-prod/project-kessel-tenant/kessel-inventory/inventory-api@sha256:3635f102ae791afb5c4056e12973ce6e56463b545c9658ca7841cf516db6807f
export MULTICLUSTER_GLOBAL_HUB_POSTGRESQL_IMAGE=registry.redhat.io/rhel9/postgresql-16@sha256:4f46bed6bce211be83c110a3452bd3f151a1e8ab150c58f2a02c56e9cc83db98

csv_name="multicluster-global-hub-operator.clusterserviceversion.yaml"
csv_file="bundle/manifests/${csv_name}"
if [ ! -f $csv_file ]; then
   echo "CSV file not found, the version or name might have changed on us!"
   exit 5
fi

# Append relatedImages to the CSV
echo -e "  relatedImages:\n" \
     " - name: multicluster-global-hub-manager\n" \
     "   image: ${MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE}\n" \
     " - name: multicluster-global-hub-agent\n" \
     "   image: ${MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE}\n" \
     " - name: grafana\n" \
     "   image: ${MULTICLUSTER_GLOBAL_HUB_GRAFANA_IMAGE}\n" \
     " - name: postgres-exporter\n" \
     "   image: ${MULTICLUSTER_GLOBAL_HUB_POSTGRES_EXPORTER_IMAGE}\n" \
     " - name: inventory-api\n" \
     "   image: ${MULTICLUSTER_GLOBAL_HUB_KESSEL_INVENTORY_API_IMAGE}\n" \
     " - name: postgresql\n" \
     "   image: ${MULTICLUSTER_GLOBAL_HUB_POSTGRESQL_IMAGE}\n" \
   >> "${csv_file}"

sed -i \
   -e "s|quay.io/stolostron/multicluster-global-hub-operator:latest|${MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE}|g" \
   -e "s|quay.io/stolostron/multicluster-global-hub-manager:latest|${MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE}|g" \
   -e "s|quay.io/stolostron/multicluster-global-hub-agent:latest|${MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE}|g" \
   -e "s|quay.io/stolostron/grafana:2.12.0-SNAPSHOT-2024-09-03-21-11-25|${MULTICLUSTER_GLOBAL_HUB_GRAFANA_IMAGE}|g" \
   -e "s|quay.io/prometheuscommunity/postgres-exporter:v0.15.0|${MULTICLUSTER_GLOBAL_HUB_POSTGRES_EXPORTER_IMAGE}|g" \
   -e "s|quay.io/stolostron/inventory-api:latest|${MULTICLUSTER_GLOBAL_HUB_KESSEL_INVENTORY_API_IMAGE}|g" \
   -e "s|quay.io/stolostron/postgresql-16:9.5-1732622748|${MULTICLUSTER_GLOBAL_HUB_POSTGRESQL_IMAGE}|g" \
	"${csv_file}"

sed -i -e "s|multicluster-global-hub-operator\\.v|multicluster-global-hub-operator-rh\\.v|g" "${csv_file}"
sed -i -e "s|multicluster-global-hub-operator|multicluster-global-hub-operator-rh|g" "bundle/metadata/annotations.yaml"
