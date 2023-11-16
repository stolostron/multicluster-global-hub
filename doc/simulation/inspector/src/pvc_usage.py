from prometheus_api_client import *
import datetime
import sys
import pandas
from colorama import Fore, Back, Style
import matplotlib.pyplot as plt
from common import *
import seaborn as sns

def check_global_hub_pvc(start_time, end_time, step):
    print(Back.GREEN+"")
    print("=============================================================================================")
    print("Checking Global Hub Components PVCs Usage")
    print("=============================================================================================")
    print(Style.RESET_ALL)  

    pc = connectProm()
    
    global_hub_total_pvc_usage(pc, start_time, end_time, step)
    global_hub_postgres_pvc_usage(pc, start_time, end_time, step)
    global_hub_kafka_pvc_usage(pc, start_time, end_time, step)

def global_hub_total_pvc_usage(pc, start_time, end_time, step):
    total = "Global Hub Total PVC Usage GB"
    file = "global-hub-total-pvc-usage"
    print(total)

    try:
        global_hub_trend = pc.custom_query_range(
        query='sum(kubelet_volume_stats_used_bytes{job="kubelet", metrics_path="/metrics", namespace="multicluster-global-hub", persistentvolumeclaim=~"postgresdb-.*"}) by (persistentvolumeclaim) / (1024*1024*1024)',
            start_time=start_time,
            end_time=end_time,
            step=step,
        )

        global_hub_trend_df = MetricRangeDataFrame(global_hub_trend)
        global_hub_trend_df["value"]=global_hub_trend_df["value"].astype(float)
        global_hub_trend_df.index= pandas.to_datetime(global_hub_trend_df.index, unit="s")
        global_hub_trend_df.rename(columns={"value": "GlobalHubPVC"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        global_hub_trend_df.plot(title=total,figsize=(figure_with, figure_hight))
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting PVC for GH: ",e)  
        print(Style.RESET_ALL)  
    print("-----------------------------------------------------------------------------------------")

def global_hub_postgres_pvc_usage(pc, start_time, end_time, step):
    title = "Global Hub Postgres PVC Usage MB"
    file = "global-hub-postgres-pvc-usage"
    print(title)
    
    try:
        query = '''
        sum(kubelet_volume_stats_used_bytes{job="kubelet", metrics_path="/metrics", namespace="multicluster-global-hub", persistentvolumeclaim=~"postgresdb-.*"}) by (persistentvolumeclaim) / (1024*1024)
        '''
        pc = connectProm()
        disk_trend = pc.custom_query_range(
            query=query,
            start_time=start_time,
            end_time=end_time,
            step=step,
        )
        
        global_hub_trend_df = MetricRangeDataFrame(disk_trend)
        global_hub_trend_df["value"]=global_hub_trend_df["value"].astype(float)
        global_hub_trend_df.index= pandas.to_datetime(global_hub_trend_df.index, unit="s")
        global_hub_trend_df.rename(columns={"value": "Storage"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='Storage', data=global_hub_trend_df, hue='persistentvolumeclaim')
        plt.title(title)
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting PVC for GH Middlewares: ",e)  
        print(Style.RESET_ALL)  
        
    print("-----------------------------------------------------------------------------------------")
    
def global_hub_kafka_pvc_usage(pc, start_time, end_time, step):
    title = "Global Hub Kafka PVC Usage MB"
    file = "global-hub-kafka-pvc-usage"
    print(title)
    
    try:
        query = '''
        sum(kubelet_volume_stats_used_bytes{job="kubelet", metrics_path="/metrics", namespace="multicluster-global-hub", persistentvolumeclaim=~"data.*-kafka-.*"}) by (persistentvolumeclaim) / (1024*1024)
        '''
        pc = connectProm()
        disk_trend = pc.custom_query_range(
            query=query,
            start_time=start_time,
            end_time=end_time,
            step=step,
        )
        
        global_hub_trend_df = MetricRangeDataFrame(disk_trend)
        global_hub_trend_df["value"]=global_hub_trend_df["value"].astype(float)
        global_hub_trend_df.index= pandas.to_datetime(global_hub_trend_df.index, unit="s")
        global_hub_trend_df.rename(columns={"value": "Storage"}, inplace = True)
        
        print(global_hub_trend_df.head(3))
        
        plt.figure(figsize=(figure_with, figure_hight))  
        sns.lineplot(x='timestamp', y='Storage', data=global_hub_trend_df, hue='persistentvolumeclaim')
        plt.title(title)
        
        plt.savefig(output_path + "/" + file + ".png")
        global_hub_trend_df.to_csv(output_path + "/" + file + ".csv", index = True, header=True)
        plt.close('all')

    except Exception as e:
        print(Fore.RED+"Error in getting PVC for GH Middlewares: ",e)  
        print(Style.RESET_ALL)  
    print("-----------------------------------------------------------------------------------------")
