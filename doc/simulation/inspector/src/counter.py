from colorama import Fore, Back, Style
from kubernetes import client, config
from datetime import datetime
import pytz
import base64
import psycopg2
import threading
import time
import pandas as pd
import os
from common import *

import matplotlib.pyplot as plt
import matplotlib.dates as mdates
from mpl_toolkits.axes_grid1 import host_subplot
from matplotlib.ticker import MaxNLocator

script_path = os.path.realpath(__file__)
output_path = os.path.join(os.path.dirname(script_path), "../output")
if not os.path.exists(output_path):
    os.makedirs(output_path)

def get_conn():
    v1 = client.CoreV1Api()
    # secret = v1.read_namespaced_secret(name="postgres-pguser-postgres", namespace=global_hub_namespace)
    secret = v1.read_namespaced_secret(name="multicluster-global-hub-postgres", namespace=global_hub_namespace)
    postgres_dict = {key: base64.b64decode(val).decode('utf-8') for key, val in secret.data.items()}
    print(postgres_dict)
    
    # need to expose the external host: 
    # kubectl patch postgrescluster postgres -p '{"spec":{"service":{"type":"LoadBalancer"}}}'  --type merge
    service = v1.read_namespaced_service(name="multicluster-global-hub-postgres-lb", namespace=global_hub_namespace)
    external_host = service.status.load_balancer.ingress[0].hostname
    
    # Create connection to postgres
    connection = psycopg2.connect(host=external_host,
                      port=5432,
                      user=postgres_dict["database-readonly-user"],
                      password=postgres_dict["database-readonly-password"],
                      dbname="hoh")
    connection.autocommit = True  # Ensure data is added to the database immediately after write commands
    # cur = connection.cursor()
    return connection

def record_initial(override):
    connection = get_conn()
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
    withHeader=True
    while True:
      try:
        cur.execute(initial_sql)
      except Exception as e:
        print("Error executing SQL query:", e)
        connection = get_conn()
        cur = connection.cursor()
        continue
      
      df = pd.DataFrame([
        {'time': datetime.now(pytz.utc).strftime("%Y-%m-%d %H:%M:%S"), 'compliance': 0, 'event': 0, 'cluster': 0},
      ])
      for record in cur.fetchall():
        df['time']=record[0]
        df[record[1]]=record[2]
        # df.loc[df['Name']==record[1], 'Time'] = record[0]
      if withHeader and override:
        df.to_csv(output_path + "/count-initialization.csv", header=True, index=False)
        withHeader = False
      else:
        df.to_csv(output_path + "/count-initialization.csv", mode='a', header=False, index=False)
        
      time.sleep(10)

def record_compliance(override):
    connection = get_conn()
    cur = connection.cursor()
    
    compliance_sql='''
    SELECT TO_CHAR(NOW(), 'YYYY-MM-DD HH24:MI:SS') as time, compliance as status, COUNT(1) as count
    FROM local_status.compliance
    GROUP BY compliance;
    '''
    withHeader=True
    while True:
      try:
        cur.execute(compliance_sql)
      except Exception as e:
        print("Error executing SQL query:", e)
        connection = get_conn()
        cur = connection.cursor()
        continue
      # cur.execute(compliance_sql)
      df = pd.DataFrame([
        {'time': datetime.now(pytz.utc).strftime("%Y-%m-%d %H:%M:%S"), 'compliant': 0, 'non_compliant': 0},
      ])
      for record in cur.fetchall():
        df['time'] = record[0]
        df[record[1]] = record[2]
        
      if withHeader and override:
        df.to_csv(output_path + "/count-compliance.csv", header=True, index=False)
        withHeader = False
      else:
        df.to_csv(output_path + "/count-compliance.csv", mode='a', header=False, index=False)
      time.sleep(60)

  
def main():
    now = datetime.now(pytz.utc)
    print(Back.LIGHTYELLOW_EX+"")
    print("************************************************************************************************")
    print("Start Global Hub Stopwatcher - ",now)
    print("************************************************************************************************")
    print(Style.RESET_ALL)
    
    override = False
    if len(sys.argv) >= 2 and sys.argv[1] == "override":
      override = True
    
    config.load_kube_config()
    
    try:
      
      thread1 = threading.Thread(target=record_initial, args=(override,))
      thread2 = threading.Thread(target=record_compliance, args=(override,))

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
    print("End Global Hub Stopwatcher")
    print("************************************************************************************************")
    print(Style.RESET_ALL)
    
def draw_intialization(df):
    plt.clf()
    plt.figure(figsize=[figure_with, figure_hight])
    host = host_subplot(111)
    twin = host.twinx()

    dates = [datetime.strptime(d, "%Y-%m-%d %H:%M:%S") for d in df['time']]
    # time,compliance,event,cluster,heartbeats
    hostline, = host.plot_date(dates, df['cluster'], '-', color='#3399e6', alpha=0.8, label="Cluster", linewidth=4.0)
    # p2, = host.plot_date(dates, df['event'], '>--', color=p1.get_color(), alpha=0.8, label="Event")
    twinline, = twin.plot_date(dates, df['compliance'], '--', color='green', alpha=0.8, label="Compliance", linewidth=2.0)

    host.legend(labelcolor="linecolor", fontsize=16)

    host.set_ylabel("Cluster", fontsize=16)
    twin.set_ylabel("Compliance", fontsize=16)

    host.yaxis.get_label().set_color(hostline.get_color())
    twin.yaxis.get_label().set_color(twinline.get_color())
    
    # make the compliance and cluster not algin
    max_cluster = df['cluster'].max()
    host.set_ylim(0, max_cluster * 1.05) 
    max_compliance = df['compliance'].max()
    twin.set_ylim(0, max_compliance * 1.1) 

    host.set_xlabel("Time", fontsize=14)
    # host.xaxis.set_major_formatter(mdates.DateFormatter("%M:%S"))
    # host.xaxis.set_major_locator(mdates.SecondLocator(interval=20))

    # host.yaxis.set_major_locator(MaxNLocator(integer=True))
    # twin.yaxis.set_major_locator(MaxNLocator(integer=True))
    
def draw():
    df = pd.read_csv(output_path + '/count-initialization.csv', on_bad_lines='skip')
    
    # data cleaning
    mask = (df['cluster'] == 0) & (df['compliance'] == 0) & (df['event'] == 0)
    indices = df[mask].index
    indices_to_remove = []
    prev_index = None
    for index in indices:
      if prev_index is not None and index != prev_index + 1:
        indices_to_remove.append(index)
      prev_index = index
    df = df.drop(indices_to_remove)
    
    # intialization - cluster compiance
    draw_intialization(df)
    
    plt.grid(axis='y', color='0.95')
    plt.title("Global Hub Managed Cluster andd Compliance Counter", fontsize=18)
    plt.savefig(output_path + '/count-initialization.png')
    
    # Initialization - event
    plt.clf()
    fig, ax = plt.subplots(figsize=(figure_with, figure_hight))
    dates = [datetime.strptime(d, "%Y-%m-%d %H:%M:%S") for d in df['time']]
    ax.plot(dates, df['event'], linewidth=3.0)
    plt.title("Global Hub Event Counter")
    plt.savefig(output_path + '/count-event.png')

    # Rotation Policy - compliance
    # time,compliant,non_compliant
    df = pd.read_csv(output_path + '/count-compliance.csv', on_bad_lines='skip')
    # data cleaning
    mask = (df['compliant'] == 0) & (df['non_compliant'] == 0)
    indices = df[mask].index
    indices_to_remove = []
    prev_index = None
    for index in indices:
      if prev_index is not None and index != prev_index + 1:
        indices_to_remove.append(index)
      prev_index = index
    df = df.drop(indices_to_remove)
    
    plt.clf()
    fig, ax = plt.subplots(figsize=(figure_with, figure_hight))
    dates = [datetime.strptime(d, "%Y-%m-%d %H:%M:%S") for d in df['time']]
    ax.plot(dates, df['compliant'], '-', color="green", label="Compliant",linewidth=2.0)
    ax.plot(dates, df['non_compliant'], '--', color="#3399e6", label="NonCompliant",linewidth=2.0)

    ax.legend(labelcolor="linecolor")

    ax.set_xlabel("Time")
    # ax.xaxis.set_major_formatter(mdates.DateFormatter("%M:%S"))
    # ax.xaxis.set_major_locator(mdates.SecondLocator(interval=20))

    # ax.yaxis.set_major_locator(MaxNLocator(integer=True))

    plt.title("Global Hub Compliance Counter")
    plt.savefig(output_path + '/count-compliance.png')

# plt.show()
    
if __name__ == "__main__":
    if len(sys.argv) >= 2 and sys.argv[1] == "draw":
      draw()
    else:
      main()