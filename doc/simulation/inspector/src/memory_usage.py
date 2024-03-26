from prometheus_api_client import *
import datetime
import sys
import pandas
from colorama import Fore, Back, Style
import matplotlib.pyplot as plt
from common import *
import seaborn as sns

def check_global_hub_memory(start_time, end_time, step):
    print(Back.GREEN+"")
    print("=============================================================================================")
    print("Checking Global Hub Components Memory Usage")
    print("=============================================================================================")
    print(Style.RESET_ALL)  
    pc = connectProm()
    
    # kubeapi_memory_usage(pc, start_time, end_time, step)
    global_hub_total_memory_usage(pc, start_time, end_time, step)
    global_hub_operator_memory_usage(pc, start_time, end_time, step)
    global_hub_manager_memory_usage(pc, start_time, end_time, step)
    global_hub_grafana_memory_usage(pc, start_time, end_time, step)
    global_hub_postgres_memory_usage(pc, start_time, end_time, step)
    global_hub_kafka_memory_usage(pc, start_time, end_time, step)
    global_hub_kafka_zookeeper_memory_usage(pc, start_time, end_time, step)
    
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
        kubeapi_trend_df = kubeapi_trend_df.sort_index(ascending=True)
        kubeapi_trend_df.rename(columns={"value": "KubeAPIMemUsageRSSGB"}, inplace = True)
        
        print(kubeapi_trend_df.head(3))
        kubeapi_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        kubeapi_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting memory (rss) for Kube API Server: ",e)    
        print(Style.RESET_ALL)
    print("-----------------------------------------------------------------------------------------")

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
        global_hub_trend_df = global_hub_trend_df.sort_index(ascending=True)
        global_hub_trend_df.rename(columns={"value": "GlobalHubMemUsageRSSGB"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("-----------------------------------------------------------------------------------------")

def global_hub_operator_memory_usage(pc, start_time, end_time, step):
    title = "Total Global Hub Operator Memory MB"
    file = "global-hub-operator-memory-usage"
    print(title)
    
    try:
        query = '''
        sum(container_memory_working_set_bytes{job="kubelet", metrics_path="/metrics/cadvisor", cluster="", namespace="multicluster-global-hub", container!="", image!="", pod=~"multicluster-global-hub-operator-.*"}) by (pod) / (1024*1024)
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
        global_hub_trend_df = global_hub_trend_df.sort_index(ascending=True)
        global_hub_trend_df.rename(columns={"value": "memory"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='memory', data=global_hub_trend_df, hue='pod')
        plt.title(title)
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("-----------------------------------------------------------------------------------------")

def global_hub_manager_memory_usage(pc, start_time, end_time, step):
    title = "Total Global Hub Manager Memory MB"
    file = "global-hub-manager-memory-usage"
    print(title)
    
    try:
        query = '''
        sum(container_memory_working_set_bytes{job="kubelet", metrics_path="/metrics/cadvisor", cluster="", namespace="multicluster-global-hub", container!="", image!="", pod=~"multicluster-global-hub-manager-.*"}) by (pod) / (1024*1024)
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
        global_hub_trend_df = global_hub_trend_df.sort_index(ascending=True)
        global_hub_trend_df.rename(columns={"value": "memory"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='memory', data=global_hub_trend_df, hue='pod')
        plt.title(title)
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("-----------------------------------------------------------------------------------------")
    

def global_hub_grafana_memory_usage(pc, start_time, end_time, step):
    title = "Total Global Hub Grafana Memory MB"
    file = "global-hub-grafana-memory-usage"
    print(title)
    
    try:
        query = '''
        sum(container_memory_working_set_bytes{job="kubelet", metrics_path="/metrics/cadvisor", cluster="", namespace="multicluster-global-hub", container!="", image!="", pod=~"multicluster-global-hub-grafana-.*"}) by (pod) / (1024*1024)
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
        global_hub_trend_df = global_hub_trend_df.sort_index(ascending=True)
        global_hub_trend_df.rename(columns={"value": "memory"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='memory', data=global_hub_trend_df, hue='pod')
        plt.title(title)
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("-----------------------------------------------------------------------------------------")
    

def global_hub_postgres_memory_usage(pc, start_time, end_time, step):
    title = "Global Hub Postgres Memory MB"
    file = "global-hub-postgres-memory-usage"
    print(title)
    
    try:
        query = '''
        sum(
            container_memory_working_set_bytes{job="kubelet", metrics_path="/metrics/cadvisor", cluster="", namespace="multicluster-global-hub", container!="", image!="", pod=~"multicluster-global-hub-postgres-.*"}
          * on(pod)
            group_left(workload, workload_type) namespace_workload_pod:kube_pod_owner:relabel{cluster="", namespace="multicluster-global-hub", workload_type="statefulset"}
        ) by (pod) / (1024 * 1024)
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
        global_hub_trend_df = global_hub_trend_df.sort_index(ascending=True)
        global_hub_trend_df.rename(columns={"value": "memory"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        # global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='memory', data=global_hub_trend_df, hue='pod')
        plt.title(title)
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("-----------------------------------------------------------------------------------------")
    
def global_hub_kafka_memory_usage(pc, start_time, end_time, step):
    title = "Global Hub Kafka Broker Memory GB"
    file = "global-hub-kafka-broker-memory-usage"
    print(title)
    
    try:
        query = '''
        sum(container_memory_working_set_bytes{job="kubelet", metrics_path="/metrics/cadvisor", cluster="", namespace="multicluster-global-hub", container!="", image!="", pod=~"kafka-kafka-.*"}) by (pod) / (1024*1024*1024)
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
        global_hub_trend_df = global_hub_trend_df.sort_index(ascending=True)
        global_hub_trend_df.rename(columns={"value": "memory"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        # global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='memory', data=global_hub_trend_df, hue='pod')
        plt.title(title)
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("-----------------------------------------------------------------------------------------")
    
def global_hub_kafka_zookeeper_memory_usage(pc, start_time, end_time, step):
    title = "Global Hub Kafka Zookeeper Memory GB"
    file = "global-hub-kafka-zookeeper-memory-usage"
    print(title)
    
    try:
        query = '''
         sum(container_memory_working_set_bytes{job="kubelet", metrics_path="/metrics/cadvisor", cluster="", namespace="multicluster-global-hub", container!="", image!="", pod=~"kafka-zookeeper-.*"}) by (pod) / (1024*1024*1024)
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
        global_hub_trend_df = global_hub_trend_df.sort_index(ascending=True)
        global_hub_trend_df.rename(columns={"value": "memory"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        # global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='memory', data=global_hub_trend_df, hue='pod')
        plt.title(title)
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH: ",e)  
        print(Style.RESET_ALL)  
    print("-----------------------------------------------------------------------------------------")

def check_global_hub_agent_memory(start_time, end_time, step):
    total = "Total Global Hub Agent Memory MB"
    file = "global-hub-agent-memory-usage"
    print(total)
    
    try:
        query = '''
        sum(
            container_memory_working_set_bytes{cluster="", namespace="multicluster-global-hub-agent", container!="", image!=""}
          * on(namespace,pod)
            group_left(workload, workload_type) namespace_workload_pod:kube_pod_owner:relabel{cluster="", namespace="multicluster-global-hub-agent", workload="multicluster-global-hub-agent", workload_type="deployment"}
        ) by (pod) / (1024*1024)
        '''
        pc = connectProm()
        memory_trend = pc.custom_query_range(
            query=query,
            start_time=start_time,
            end_time=end_time,
            step=step,
        )

        memory_trend_df = MetricRangeDataFrame(memory_trend)
        memory_trend_df["value"]=memory_trend_df["value"].astype(float)
        memory_trend_df.index= pandas.to_datetime(memory_trend_df.index, unit="s")
        memory_trend_df = memory_trend_df.sort_index(ascending=True)
        memory_trend_df.rename(columns={"value": "Usage"}, inplace = True)
        
        print(memory_trend_df.head(3))
        memory_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        memory_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting Memory (rss) for GH Agent: ",e)  
        print(Style.RESET_ALL)  
    print("-----------------------------------------------------------------------------------------")