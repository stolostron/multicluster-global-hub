from kubernetes import client, config
from prometheus_api_client import *
import sys

global_hub_namespace="multicluster-global-hub"

def connectProm():
  try:
    custom_object_api = client.CustomObjectsApi()
    prom_route = custom_object_api.get_namespaced_custom_object(
                  "route.openshift.io", "v1", "openshift-monitoring", "routes", "thanos-querier")
    prom_url = "https://{}".format(prom_route['spec']['host'])
    
    # Get Kubernetes API token.
    c = client.Configuration()
    config.load_config(client_configuration = c)
    api_token = c.api_key['authorization']
    pc = PrometheusConnect(url=prom_url, headers={"Authorization": "{}".format(api_token)}, disable_ssl=True)
  except Exception as e:
      print("Failure: ",e) 
      sys.exit("Is PROM_URL, API_TOKEN env variables defined or are they accurate")  
      
  return pc