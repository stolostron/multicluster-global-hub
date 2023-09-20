from datetime import datetime, timedelta
import pytz
from colorama import Back, Style
import sys

# pass debug(boolean) as env
def main():
    start_time=(datetime.now(pytz.utc) - timedelta(days=7))
    end_time=datetime.now(pytz.utc)
    step='1m'
    
    print(Back.LIGHTYELLOW_EX+"")
    print("************************************************************************************************")
    print("Starting date for the Global Hub Health Check  - ", datetime.now(pytz.utc))
    print("Starting datetime for History collection - ", start_time)
    print("End date and time for History collection - ", end_time)
    print("************************************************************************************************")
    print(Style.RESET_ALL)
   
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
    print("End Global Hub Health Check")
    print("************************************************************************************************")
    print(Style.RESET_ALL)
    
if __name__ == "__main__":
    main()