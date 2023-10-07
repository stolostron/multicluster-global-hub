from datetime import datetime, timedelta
import pytz
from colorama import Back, Style
import sys

from cpu_usage import *
from memory_usage import *

# pass debug(boolean) as env
def main():
    start_time=(datetime.now(pytz.utc) - timedelta(days=7))
    end_time=datetime.now(pytz.utc)
    step='1m'
    
    try:
      if len(sys.argv) >= 2 and sys.argv[1] != "":
        start_time = datetime.strptime(sys.argv[1], "%Y-%m-%d %H:%M:%S")
      if len(sys.argv) >= 3 and sys.argv[2] != "":
        end_time = datetime.strptime(sys.argv[2], "%Y-%m-%d %H:%M:%S")
    except ValueError:
      print("Invalid datetime format. Please use 'YYYY-MM-DD HH:MM:SS'.", sys.argv)

    
    print(Back.LIGHTYELLOW_EX+"")
    print("************************************************************************************************")
    print("Starting date for the Global Hub Agent Health Check  - ", datetime.now(pytz.utc))
    print("Starting history collection from", start_time, "to", end_time)
    print("************************************************************************************************")
    print(Style.RESET_ALL)
   
    config.load_kube_config()
    check_global_hub_agent_cpu(start_time, end_time, step)
    check_global_hub_agent_memory(start_time, end_time, step)

    print(Back.LIGHTYELLOW_EX+"")
    print("************************************************************************************************")
    print("End Global Hub Agent Health Check")
    print("************************************************************************************************")
    print(Style.RESET_ALL)
    
if __name__ == "__main__":
    main()