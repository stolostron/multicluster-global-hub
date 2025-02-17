#! /bin/bash

set -e

export MULTICLUSTER_GLOBAL_HUB_AGENT_IMAGE=quay.io/redhat-user-workloads/acm-multicluster-glo-tenant/multicluster-global-hub-agent-globalhub-1-4@sha256:8deff677f5a42ac4a85425cb889fe5c520cf1caebc6ea1fd86eefd566997b1be
export MULTICLUSTER_GLOBAL_HUB_MANAGER_IMAGE=quay.io/redhat-user-workloads/acm-multicluster-glo-tenant/multicluster-global-hub-manager-globalhub-1-4@sha256:ed275d6600d9896b601cdf0fd1c52c827f1bcad12356b539c42674dba5731fc2
export MULTICLUSTER_GLOBAL_HUB_OPERATOR_IMAGE=quay.io/redhat-user-workloads/acm-multicluster-glo-tenant/multicluster-global-hub-operator-globalhub-1-4@sha256:ecf3fe709844ef6d92768cd5507f5909ca87d9f2ac187e56b0fdf0535c112b12
export MULTICLUSTER_GLOBAL_HUB_GRAFANA_IMAGE=quay.io/redhat-user-workloads/acm-multicluster-glo-tenant/glo-grafana-globalhub-1-4@sha256:d4b73f8f8e4db0d987a1b177d8513be589fdf048f843a43690b0e3ff52963bba
export MULTICLUSTER_GLOBAL_HUB_POSTGRES_EXPORTER_IMAGE=quay.io/redhat-user-workloads/acm-multicluster-glo-tenant/postgres-exporter-globalhub-1-4@sha256:d52c54efdff996c765f2cf2e6189e57ae4133c5840d1f0623319401cb6456b14
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
