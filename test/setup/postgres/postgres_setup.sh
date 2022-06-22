
# source this script to enable the postgres in hub-of-hubs. DATABASE_URL_HOH and DATABASE_URL_HOH could be used to init hub-of-hubs

function deployComponent() {
  # deploy component
  # before: checkout code
  rm -rf $1
  git clone https://github.com/stolostron/$1.git
  cd $1
  git checkout $2
  # action
  $3
  # after
  cd ..
  rm -rf $1
}

function deployHohPostgresqlAction() {
  cd ./pgo
  IMAGE=quay.io/open-cluster-management-hub-of-hubs/postgresql-ansible:$TAG ./setup.sh
  cd ..
}

function deployPostgres() {
  # always check whether DATABASE_URL_HOH and DATABASE_URL_TRANSPORT are set, if not - install PGO and use its secrets
  if [ -z "${DATABASE_URL_HOH-}" ] && [ -z "${DATABASE_URL_TRANSPORT-}" ]; then
    deployComponent "hub-of-hubs-postgresql" "$branch" deployHohPostgresqlAction

    pg_namespace="hoh-postgres"
    process_user="hoh-pguser-hoh-process-user"
    transport_user="hoh-pguser-transport-bridge-user"

    DATABASE_URL_HOH="$(kubectl get secrets -n "${pg_namespace}" "${process_user}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
    DATABASE_URL_HOH="$(kubectl get secrets -n "${pg_namespace}" "${transport_user}" -o go-template='{{index (.data) "pgbouncer-uri" | base64decode}}')"
  fi
}

deployPostgres
