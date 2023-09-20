from common import *       
from colorama import Fore, Back, Style
from kubernetes import client, config
from datetime import datetime
import base64
import psycopg2

# pass debug(boolean) as env
def main():

    # start_time=(datetime.now() - timedelta(days=7))
    # end_time=datetime.now()
    # #start_time=dt.datetime(2021, 7, 31, 21, 30, 0, tzinfo=query.getUTC())
    # #end_time=dt.datetime(2021, 8, 1, 12, 25, 0, tzinfo=query.getUTC())
    # step='1m'

    now = datetime.now()
    print(Back.LIGHTYELLOW_EX+"")
    print("************************************************************************************************")
    print("Start GH Stopwatcher - ",now)
    print("************************************************************************************************")
    print(Style.RESET_ALL)
    
    config.load_kube_config()
    
    v1 = client.CoreV1Api()
    
    try:
      
      secret = v1.read_namespaced_secret(name="postgres-pguser-postgres", namespace=global_hub_namespace)
      postgres_dict = {key: base64.b64decode(val).decode('utf-8') for key, val in secret.data.items()}
      
      # need to expose the external host: 
      # kubectl patch postgrescluster postgres -p '{"spec":{"service":{"type":"LoadBalancer"}}}'  --type merge
      service = v1.read_namespaced_service(name="postgres-ha", namespace=global_hub_namespace)
      external_host = service.status.load_balancer.ingress[0].hostname
      
      # Create connection to postgres
      connection = psycopg2.connect(host=external_host,
                        port=postgres_dict["port"],
                        user=postgres_dict["user"],
                        password=postgres_dict["password"],
                        dbname=postgres_dict["dbname"])
      connection.autocommit = True  # Ensure data is added to the database immediately after write commands
      
      # print(postgres_dict)
      
      cur = connection.cursor()
      
      initial_sql='''
      SELECT TO_CHAR(NOW(), 'YYYY-MM-DD HH24:MI:SS') as time, table_name, COUNT(1) AS count
      FROM (
          SELECT 'cluster' AS table_name FROM status.managed_clusters
          UNION ALL
          SELECT 'compliance' AS table_name FROM local_status.compliance
          UNION ALL
          SELECT 'event' AS table_name FROM "event".local_policies
      ) AS subquery
      GROUP BY table_name;
      '''
      
      cur.execute(initial_sql)
      
      print(cur.fetchall())
      cur.close()
      connection.close()

   


    
    except client.exceptions.ApiException as e:
      print(f"Error fetching the secret: {e}")

    # namespace = "your-namespace"
    # secret_name = "your-secret-name"
   
    # createSubdir()
    # mch = checkMCHStatus()
    # node = checkNodeStatus()

    # if tsdb == "prom" : #if route is cluster prom
    #      cont = checkACMContainerStatus(start_time, end_time, step)
    #      api = checkAPIServerStatus(start_time, end_time, step)
    #      etcd = checkEtcdStatus(start_time, end_time, step)
    #      cpu = checkCPUUsage(start_time, end_time, step)
    #      memory = checkMemoryUsage(start_time, end_time, step)
    #      thanos = checkThanosStatus(start_time, end_time, step)
    #      apiObjet = checkAPIServerObjects(start_time, end_time, step)
    # else: #if route is observability thanos
    #      # does not work yet
    #      sizing = checkACMHubClusterUtilization() 
    
    # mc = checkManagedClusterStatus()
    # getManagedClusterNodeCount()
    # saveMasterDF()

    print(Back.LIGHTYELLOW_EX+"")
    print("************************************************************************************************")
    print("End GH")
    print("************************************************************************************************")
    print(Style.RESET_ALL)
    
if __name__ == "__main__":
    main()