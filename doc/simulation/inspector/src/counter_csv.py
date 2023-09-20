from common import *       
from colorama import Fore, Back, Style
from kubernetes import client, config
from datetime import datetime
import pytz
import base64
import psycopg2
import threading
import time
import warnings
import pandas as pd
import os
warnings.filterwarnings("ignore")

script_path = os.path.realpath(__file__)
output_path = os.path.join(os.path.dirname(script_path), "../output")
if not os.path.exists(output_path):
    os.makedirs(output_path)

def record_initial(cur):
    initial_sql='''
    SELECT TO_CHAR(NOW(), 'YYYY-MM-DD HH24:MI:SS') as time, table_name, COUNT(1) AS count
    FROM (
        SELECT 'cluster' AS table_name FROM status.managed_clusters
        UNION ALL
        SELECT 'compliance' AS table_name FROM local_status.compliance
        UNION ALL
        SELECT 'event' AS table_name FROM "event".local_policies
        UNION ALL
        SELECT 'heartbeats' AS table_name from status.leaf_hub_heartbeats
    ) AS subquery
    GROUP BY table_name;
    '''
    withHeader=True
    while True:
      cur.execute(initial_sql)
      df = pd.DataFrame([
        {'time': datetime.now(pytz.utc).strftime("%Y-%m-%d %H:%M:%S"), 'compliance': 0, 'event': 0, 'cluster': 0, 'heartbeats': 0},
      ])
      for record in cur.fetchall():
        df['time']=record[0]
        df[record[1]]=record[2]
        # df.loc[df['Name']==record[1], 'Time'] = record[0]
      if withHeader:
        df.to_csv(output_path + "/count-initialization.csv", header=True, index=False)
        withHeader = False
      else:
        df.to_csv(output_path + "/count-initialization.csv", mode='a', header=False, index=False)
        
      time.sleep(10)

def record_compliance(cur):
    compliance_sql='''
    SELECT TO_CHAR(NOW(), 'YYYY-MM-DD HH24:MI:SS') as time, compliance as status, COUNT(1) as count
    FROM local_status.compliance
    GROUP BY compliance;
    '''
    withHeader=True
    while True:
      cur.execute(compliance_sql)
      df = pd.DataFrame([
        {'time': datetime.now(pytz.utc).strftime("%Y-%m-%d %H:%M:%S"), 'compliant': 0, 'non_compliant': 0},
      ])
      for record in cur.fetchall():
        df['time'] = record[0]
        df[record[1]] = record[2]
        
      if withHeader:
        df.to_csv(output_path + "/count-compliance.csv", header=True, index=False)
        withHeader = False
      else:
        df.to_csv(output_path + "/count-compliance.csv", mode='a', header=False, index=False)
      time.sleep(60)

def main():
    now = datetime.now(pytz.utc)
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
      cur = connection.cursor()
      thread1 = threading.Thread(target=record_initial, args=(cur,))
      thread2 = threading.Thread(target=record_compliance, args=(cur,))

      thread1.start()
      thread2.start()

      thread1.join()
      thread2.join()
  
      cur.close()
      connection.close()

    except Exception as e:
      print(Fore.RED+"Error to stopwatch the database: ",e)    
      print(Style.RESET_ALL)

    print(Back.LIGHTYELLOW_EX+"")
    print("************************************************************************************************")
    print("End GH Stopwatcher")
    print("************************************************************************************************")
    print(Style.RESET_ALL)
    
if __name__ == "__main__":
    main()