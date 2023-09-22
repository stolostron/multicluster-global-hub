from prometheus_api_client import *
import datetime
import sys
import pandas
from colorama import Fore, Back, Style
import matplotlib.pyplot as plt
from common import *

def check_global_hub_memory(start_time, end_time, step):
    print(Back.LIGHTYELLOW_EX+"")
    print("************************************************************************************************")
    print("Checking Memory Usage")
    print("************************************************************************************************")
    print(Style.RESET_ALL)
    pc = connectProm()
    
    kubeapi_memory_usage(pc, start_time, end_time, step)
    global_hub_total_memory_usage(pc, start_time, end_time, step)
    global_hub_operator_memory_usage(pc, start_time, end_time, step)
    global_hub_manager_memory_usage(pc, start_time, end_time, step)
    global_hub_grafana_memory_usage(pc, start_time, end_time, step)
    global_hub_postgres_memory_usage(pc, start_time, end_time, step)
    global_hub_kafka_memory_usage(pc, start_time, end_time, step)
    
    print(Back.LIGHTYELLOW_EX+"")
    print("************************************************************************************************")
    print("Memory Health Check  - ", "PLEASE CHECK to see if the results are concerning!! ")
    print("************************************************************************************************")
    print(Style.RESET_ALL)

def kubeapi_memory_usage(pc, start_time, end_time, step):

    title = "Total Kube API Server Memory(RSS) GB"
    file = "kubeapi-memory-usage"
    print(title)

    try:
        kubeapi_trend = pc.custom_query_range(
        query='sum(container_memory_rss{job="kubelet", metrics_path="/metrics/cadvisor", cluster="", container!="", namespace=~"openshift-kube-apiserver|openshift-etcd"})/(1024*1024*1024)',
            start_time=start_time,
            end_time=end_time,
            step=step,
        )

        kubeapi_trend_df = MetricRangeDataFrame(kubeapi_trend)
        kubeapi_trend_df["value"]=kubeapi_trend_df["value"].astype(float)
        kubeapi_trend_df.index= pandas.to_datetime(kubeapi_trend_df.index, unit="s")
        
        kubeapi_trend_df.rename(columns={"value": "KubeAPIMemUsageRSSGB"}, inplace = True)
        
        print(kubeapi_trend_df.head(3))
        kubeapi_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        kubeapi_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting memory (rss) for Kube API Server: ",e)    
        print(Style.RESET_ALL)
    print("=============================================")

def global_hub_total_memory_usage(pc, start_time, end_time, step):
    total = "Total Global Hub Total Memory GB"
    file = "global-hub-total-memory-usage"
    print(total)

    try:
        global_hub_trend = pc.custom_query_range(
        query='sum(container_memory_rss{job="kubelet", metrics_path="/metrics/cadvisor", cluster="", container!="", namespace=~"multicluster-global-hub"})/(1024*1024*1024)',
            start_time=start_time,
            end_time=end_time,
            step=step,
        )

        global_hub_trend_df = MetricRangeDataFrame(global_hub_trend)
        global_hub_trend_df["value"]=global_hub_trend_df["value"].astype(float)
        global_hub_trend_df.index= pandas.to_datetime(global_hub_trend_df.index, unit="s")
        global_hub_trend_df.rename(columns={"value": "GlobalHubMemUsageRSSGB"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("=============================================")

def global_hub_operator_memory_usage(pc, start_time, end_time, step):
    total = "Total Global Hub Operator Memory MB"
    file = "global-hub-operator-memory-usage"
    print(total)
    
    try:
        query = '''
        sum(
            container_memory_working_set_bytes{cluster="", namespace="multicluster-global-hub", container!="", image!=""}
          * on(namespace,pod)
            group_left(workload, workload_type) namespace_workload_pod:kube_pod_owner:relabel{cluster="", namespace="multicluster-global-hub", workload="multicluster-global-hub-operator", workload_type="deployment"}
        ) by (pod) / (1024*1024)
        '''
        global_hub_trend = pc.custom_query_range(
            query=query,
            start_time=start_time,
            end_time=end_time,
            step=step,
        )

        global_hub_trend_df = MetricRangeDataFrame(global_hub_trend)
        global_hub_trend_df["value"]=global_hub_trend_df["value"].astype(float)
        global_hub_trend_df.index= pandas.to_datetime(global_hub_trend_df.index, unit="s")
        global_hub_trend_df.rename(columns={"value": "Usage"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("=============================================")

def global_hub_manager_memory_usage(pc, start_time, end_time, step):
    total = "Total Global Hub Manager Memory MB"
    file = "global-hub-manager-memory-usage"
    print(total)
    
    try:
        query = '''
        sum(
            container_memory_working_set_bytes{cluster="", namespace="multicluster-global-hub", container!="", image!=""}
          * on(namespace,pod)
            group_left(workload, workload_type) namespace_workload_pod:kube_pod_owner:relabel{cluster="", namespace="multicluster-global-hub", workload="multicluster-global-hub-manager", workload_type="deployment"}
        ) by (pod) / (1024*1024)
        '''
        global_hub_trend = pc.custom_query_range(
            query=query,
            start_time=start_time,
            end_time=end_time,
            step=step,
        )

        global_hub_trend_df = MetricRangeDataFrame(global_hub_trend)
        global_hub_trend_df["value"]=global_hub_trend_df["value"].astype(float)
        global_hub_trend_df.index= pandas.to_datetime(global_hub_trend_df.index, unit="s")
        global_hub_trend_df.rename(columns={"value": "Usage"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("=============================================")
    

def global_hub_grafana_memory_usage(pc, start_time, end_time, step):
    total = "Total Global Hub Grafana Memory MB"
    file = "global-hub-grafana-memory-usage"
    print(total)
    
    try:
        query = '''
        sum(
            container_memory_working_set_bytes{cluster="", namespace="multicluster-global-hub", container!="", image!=""}
          * on(namespace,pod)
            group_left(workload, workload_type) namespace_workload_pod:kube_pod_owner:relabel{cluster="", namespace="multicluster-global-hub", workload="multicluster-global-hub-grafana", workload_type="deployment"}
        ) by (pod) / (1024*1024)
        '''
        global_hub_trend = pc.custom_query_range(
            query=query,
            start_time=start_time,
            end_time=end_time,
            step=step,
        )

        global_hub_trend_df = MetricRangeDataFrame(global_hub_trend)
        global_hub_trend_df["value"]=global_hub_trend_df["value"].astype(float)
        global_hub_trend_df.index= pandas.to_datetime(global_hub_trend_df.index, unit="s")
        global_hub_trend_df.rename(columns={"value": "Usage"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("=============================================")
    

def global_hub_postgres_memory_usage(pc, start_time, end_time, step):
    total = "Average Global Hub Postgres Memory MB"
    file = "global-hub-postgres-memory-usage"
    print(total)
    
    try:
        # replicas = 3
        query = '''
          sum(
            container_memory_usage_bytes{namespace="multicluster-global-hub", pod=~"postgres-pgha.*"}
          ) by (namespace, pod) / (1024*1024) / 3
        '''
        global_hub_trend = pc.custom_query_range(
            query=query,
            start_time=start_time,
            end_time=end_time,
            step=step,
        )

        global_hub_trend_df = MetricRangeDataFrame(global_hub_trend)
        global_hub_trend_df["value"]=global_hub_trend_df["value"].astype(float)
        global_hub_trend_df.index= pandas.to_datetime(global_hub_trend_df.index, unit="s")
        global_hub_trend_df.rename(columns={"value": "Usage"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("=============================================")
    
def global_hub_kafka_memory_usage(pc, start_time, end_time, step):
    total = "Average Global Hub Kafka Memory MB"
    file = "global-hub-kafka-memory-usage"
    print(total)
    
    try:
        # replicas = 3
        query = '''
          sum(
            container_memory_usage_bytes{namespace="multicluster-global-hub", pod=~"kafka-kafka-.*"}
          ) by (namespace, pod) / (1024*1024) / 3
        '''
        global_hub_trend = pc.custom_query_range(
            query=query,
            start_time=start_time,
            end_time=end_time,
            step=step,
        )

        global_hub_trend_df = MetricRangeDataFrame(global_hub_trend)
        global_hub_trend_df["value"]=global_hub_trend_df["value"].astype(float)
        global_hub_trend_df.index= pandas.to_datetime(global_hub_trend_df.index, unit="s")
        global_hub_trend_df.rename(columns={"value": "Usage"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("=============================================")