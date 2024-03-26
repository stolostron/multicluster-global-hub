from prometheus_api_client import *
import datetime
import sys
import numpy as np
import pandas
from common import *
from colorama import Fore, Back, Style
import matplotlib.pyplot as plt
import seaborn as sns

def check_global_hub_cpu(start_time, end_time, step):
    print(Back.GREEN+"")
    print("=============================================================================================")
    print("Checking Global Hub Components CPU Usage")
    print("=============================================================================================")
    print(Style.RESET_ALL)  
    
    pc=connectProm()
    
    kubeapi_cpu_usage(pc, start_time, end_time, step)
    global_hub_total(pc, start_time, end_time, step)
    global_hub_operator(pc, start_time, end_time, step)
    global_hub_manager(pc, start_time, end_time, step)
    global_hub_grafana(pc, start_time, end_time, step)
    global_hub_postgres(pc, start_time, end_time, step)
    global_hub_kafka(pc, start_time, end_time, step)
    global_hub_zookeeper_kafka(pc, start_time, end_time, step)
    
def kubeapi_cpu_usage(pc, start_time, end_time, step):
    file = 'kubeapi-cpu-usage'
    title = 'Total Kube API Server CPU Core usage'
    print(title)

    try:
        query = '''sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace=~"openshift-kube-apiserver|openshift-etcd"})
        '''
        kubeapi_cpu = pc.custom_query(query)

        kubeapi_cpu_df = MetricSnapshotDataFrame(kubeapi_cpu)
        kubeapi_cpu_df["value"]=kubeapi_cpu_df["value"].astype(float)
        kubeapi_cpu_df.rename(columns={"value": "KubeAPICPUCoreUsage"}, inplace = True)
        # print(kubeapi_cpu_df.to_markdown())

        kubeapi_cpu_trend = pc.custom_query_range(
        query=query,
            start_time=start_time,
            end_time=end_time,
            step=step,
        )

        kubeapi_cpu_trend_df = MetricRangeDataFrame(kubeapi_cpu_trend)
        kubeapi_cpu_trend_df["value"]=kubeapi_cpu_trend_df["value"].astype(float)

        kubeapi_cpu_trend_df.index= pandas.to_datetime(kubeapi_cpu_trend_df.index, unit="s")
        kubeapi_cpu_trend_df = kubeapi_cpu_trend_df.sort_index(ascending=True)
        kubeapi_cpu_trend_df.rename(columns={"value": "KubeAPICPUCoreUsage"}, inplace = True)
        print(kubeapi_cpu_trend_df.head(3))
        kubeapi_cpu_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        plt.savefig(output_path + "/" + file + ".png")
        kubeapi_cpu_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting cpu for Kube API Server: ",e) 
        print(Style.RESET_ALL) 
          
    print("-----------------------------------------------------------------------------------------")
   
def global_hub_operator(pc, start_time, end_time, step):
    file = 'global-hub-operator-cpu-usage'
    title = 'Global Hub Operator CPU'
    print(title)
    try:
        query = '''
          sum(
            node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{cluster="",namespace="multicluster-global-hub"}
            * on(namespace,pod)
            group_left(workload, workload_type) namespace_workload_pod:kube_pod_owner:relabel{cluster="", namespace="multicluster-global-hub", workload="multicluster-global-hub-operator", workload_type="deployment"}
          ) by (pod)
        '''
        # operator_cpu = pc.custom_query(query)
        # operator_cpu_df = MetricSnapshotDataFrame(operator_cpu)
        # operator_cpu_df["value"]=operator_cpu_df["value"].astype(float)
        # operator_cpu_df.rename(columns={"value": "Usage"}, inplace = True)
        # print(operator_cpu_df.to_markdown())

        operator_cpu_trend = pc.custom_query_range(
          query=query,
          start_time=start_time,
          end_time=end_time,
          step=step,
        )

        operator_cpu_trend_df = MetricRangeDataFrame(operator_cpu_trend)
        operator_cpu_trend_df["value"]=operator_cpu_trend_df["value"].astype(float)
        operator_cpu_trend_df.index= pandas.to_datetime(operator_cpu_trend_df.index, unit="s")
        operator_cpu_trend_df = operator_cpu_trend_df.sort_index(ascending=True)
        
        #node_cpu_trend_df =  node_cpu_trend_df.pivot( columns='node',values='value')
        operator_cpu_trend_df.rename(columns={"value": "Usage"}, inplace = True)
        print(operator_cpu_trend_df.head(3))
        operator_cpu_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        plt.savefig(output_path + "/" + file + ".png")
        operator_cpu_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')
        
    except Exception as e:
        print(Fore.RED+"Error in getting cpu for Global Hub Operator: ",e) 
        print(Style.RESET_ALL)   
    print("-----------------------------------------------------------------------------------------")

  
def global_hub_manager(pc, start_time, end_time, step):
    file = 'global-hub-manager-cpu-usage'
    title = 'Global Hub Manager CPU'
    print(title)
    try:
        query = '''
        sum(
          node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="multicluster-global-hub", pod=~"multicluster-global-hub-manager-.*"}
        ) by (pod)
        '''
        cpu_trend = pc.custom_query_range(
          query=query,
          start_time=start_time,
          end_time=end_time,
          step=step,
        )

        cpu_trend_df = MetricRangeDataFrame(cpu_trend)
        cpu_trend_df["value"]=cpu_trend_df["value"].astype(float)
        cpu_trend_df.index= pandas.to_datetime(cpu_trend_df.index, unit="s")
        cpu_trend_df = cpu_trend_df.sort_index(ascending=True)
        cpu_trend_df.rename(columns={"value": "cpu"}, inplace = True)
        
        print(cpu_trend_df.head(3))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='cpu', data=cpu_trend_df, hue='pod')
        plt.title(title)
        # cpu_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        plt.savefig(output_path + "/" + file + ".png")
        cpu_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')
    except Exception as e:
        print(Fore.RED+"Error in getting CPU: ",e) 
        print(Style.RESET_ALL) 
          
    print("-----------------------------------------------------------------------------------------")

def global_hub_grafana(pc, start_time, end_time, step):
    file = 'global-hub-grafana-cpu-usage'
    title = 'Global Hub Grafana CPU'
    print(title)
    try:
        query = '''
        sum(
          node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="multicluster-global-hub", pod=~"multicluster-global-hub-grafana-.*"}
        ) by (pod)
        '''
        cpu_trend = pc.custom_query_range(
          query=query,
          start_time=start_time,
          end_time=end_time,
          step=step,
        )

        cpu_trend_df = MetricRangeDataFrame(cpu_trend)
        cpu_trend_df["value"]=cpu_trend_df["value"].astype(float)
        cpu_trend_df.index= pandas.to_datetime(cpu_trend_df.index, unit="s")
        cpu_trend_df = cpu_trend_df.sort_index(ascending=True)
        cpu_trend_df.rename(columns={"value": "cpu"}, inplace = True)
        
        print(cpu_trend_df.head(3))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='cpu', data=cpu_trend_df, hue='pod')
        plt.title(title)
        # cpu_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        plt.savefig(output_path + "/" + file + ".png")
        cpu_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')
        
    except Exception as e:
        print(Fore.RED+"Error in getting CPU: ",e) 
        print(Style.RESET_ALL) 
          
    print("-----------------------------------------------------------------------------------------")

def global_hub_total(pc, start_time, end_time, step):
    file = 'global-hub-total-cpu-usage'
    title = 'Global Hub CPU Core Usage'
    print(title)
    try:
        query = '''
          sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="multicluster-global-hub"})
        '''
        cpu_trend = pc.custom_query_range(
          query=query,
          start_time=start_time,
          end_time=end_time,
          step=step,
        )

        cpu_trend_df = MetricRangeDataFrame(cpu_trend)
        cpu_trend_df["value"]=cpu_trend_df["value"].astype(float)
        cpu_trend_df.index= pandas.to_datetime(cpu_trend_df.index, unit="s")
        cpu_trend_df = cpu_trend_df.sort_index(ascending=True)
        cpu_trend_df.rename(columns={"value": "Usage"}, inplace = True)
        
        print(cpu_trend_df.head(3))
        cpu_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        plt.savefig(output_path + "/" + file + ".png")
        cpu_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')
    except Exception as e:
        print(Fore.RED+"Error in getting CPU: ",e) 
        print(Style.RESET_ALL) 
          
    print("-----------------------------------------------------------------------------------------")

def global_hub_postgres(pc, start_time, end_time, step):
    file = 'global-hub-postgres-cpu-usage'
    title = 'Global Hub Postgres CPU'
    print(title)
    try:
        query = '''
        sum(
          node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="multicluster-global-hub", pod=~"multicluster-global-hub-postgres.*"}
        ) by (pod)
        '''
        cpu_trend = pc.custom_query_range(
          query=query,
          start_time=start_time,
          end_time=end_time,
          step=step,
        )

        cpu_trend_df = MetricRangeDataFrame(cpu_trend)
        cpu_trend_df["value"]=cpu_trend_df["value"].astype(float)
        cpu_trend_df.index= pandas.to_datetime(cpu_trend_df.index, unit="s")
        cpu_trend_df = cpu_trend_df.sort_index(ascending=True)
        cpu_trend_df.rename(columns={"value": "cpu"}, inplace = True)
        
        print(cpu_trend_df.head(3))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='cpu', data=cpu_trend_df, hue='pod')
        plt.title(title)
        # cpu_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        plt.savefig(output_path + "/" + file + ".png")
        cpu_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')
    except Exception as e:
        print(Fore.RED+"Error in getting CPU: ",e) 
        print(Style.RESET_ALL) 
          
    print("-----------------------------------------------------------------------------------------")

def global_hub_kafka(pc, start_time, end_time, step):
    file = 'global-hub-kafka-broker-cpu-usage'
    title = 'Global Hub Kafka broker CPU'
    print(title)
    try:
        query = '''
        sum(
          node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="multicluster-global-hub", pod=~"kafka-kafka-.*"}
        ) by (pod)
        '''
        cpu_trend = pc.custom_query_range(
          query=query,
          start_time=start_time,
          end_time=end_time,
          step=step,
        )

        cpu_trend_df = MetricRangeDataFrame(cpu_trend)
        cpu_trend_df["value"]=cpu_trend_df["value"].astype(float)
        cpu_trend_df.index= pandas.to_datetime(cpu_trend_df.index, unit="s")
        cpu_trend_df = cpu_trend_df.sort_index(ascending=True)
        cpu_trend_df.rename(columns={"value": "cpu"}, inplace = True)
        
        print(cpu_trend_df.head(3))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='cpu', data=cpu_trend_df, hue='pod')
        plt.title(title)
        # cpu_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        plt.savefig(output_path + "/" + file + ".png")
        cpu_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')
    except Exception as e:
        print(Fore.RED+"Error in getting CPU: ",e) 
        print(Style.RESET_ALL) 
          
    print("-----------------------------------------------------------------------------------------")
    
def global_hub_zookeeper_kafka(pc, start_time, end_time, step):
    file = 'global-hub-kafka-zookeeper-cpu-usage'
    title = 'Global Hub Kafka zookeeper CPU'
    print(title)
    try:
        query = '''
        sum(
          node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="multicluster-global-hub", pod=~"kafka-zookeeper-.*"}
        ) by (pod)
        '''
        cpu_trend = pc.custom_query_range(
          query=query,
          start_time=start_time,
          end_time=end_time,
          step=step,
        )

        cpu_trend_df = MetricRangeDataFrame(cpu_trend)
        cpu_trend_df["value"]=cpu_trend_df["value"].astype(float)
        cpu_trend_df.index= pandas.to_datetime(cpu_trend_df.index, unit="s")
        cpu_trend_df = cpu_trend_df.sort_index(ascending=True)
        cpu_trend_df.rename(columns={"value": "cpu"}, inplace = True)
        
        print(cpu_trend_df.head(3))
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='cpu', data=cpu_trend_df, hue='pod')
        plt.title(title)
        # cpu_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        plt.savefig(output_path + "/" + file + ".png")
        cpu_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')
    except Exception as e:
        print(Fore.RED+"Error in getting CPU: ",e) 
        print(Style.RESET_ALL) 
          
    print("-----------------------------------------------------------------------------------------")


def check_global_hub_agent_cpu(start_time, end_time, step):
    file = 'global-hub-agent-cpu-usage'
    title = 'Global Hub Agent CPU'
    print(title)
    try:
        query = '''
          sum(
            node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{cluster="",namespace="multicluster-global-hub-agent"}
            * on(namespace,pod)
            group_left(workload, workload_type) namespace_workload_pod:kube_pod_owner:relabel{cluster="", namespace="multicluster-global-hub-agent", workload="multicluster-global-hub-agent", workload_type="deployment"}
          ) by (pod)
        '''

        pc=connectProm()
        cpu_trend = pc.custom_query_range(
          query=query,
          start_time=start_time,
          end_time=end_time,
          step=step,
        )

        cpu_trend_df = MetricRangeDataFrame(cpu_trend)
        cpu_trend_df["value"]=cpu_trend_df["value"].astype(float)
        cpu_trend_df.index= pandas.to_datetime(cpu_trend_df.index, unit="s")
        cpu_trend_df = cpu_trend_df.sort_index(ascending=True)
        
        #node_cpu_trend_df =  node_cpu_trend_df.pivot( columns='node',values='value')
        cpu_trend_df.rename(columns={"value": "Usage"}, inplace = True)
        print(cpu_trend_df.head(3))
        cpu_trend_df.plot(title=title,figsize=(figure_with, figure_hight))
        plt.savefig(output_path + "/" + file + ".png")
        cpu_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')
    except Exception as e:
        print(Fore.RED+"Error in getting cpu for Global Hub Agent: ",e) 
        print(Style.RESET_ALL)   
        
    print("-----------------------------------------------------------------------------------------")
