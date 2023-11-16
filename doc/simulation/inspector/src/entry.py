from datetime import datetime, timedelta
import pytz
from colorama import Back, Style
import sys

from cpu_usage import *
from memory_usage import *
from pvc_usage import *

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

    
    print(Back.BLUE+"")
    current = datetime.now().strftime("%H:%M:%S")
    print("***********************************************************************************************************")
    print("[ {} ] Start Global Hub Inspector: [ {} | {} | {} ]".format(current, start_time, end_time, step))
    print("***********************************************************************************************************")
    print(Style.RESET_ALL)
   
    config.load_kube_config()
    check_global_hub_cpu(start_time, end_time, step)
    check_global_hub_memory(start_time, end_time, step)
    check_global_hub_pvc(start_time, end_time, step)

    print(Back.BLUE+"")
    current = datetime.now().strftime("%H:%M:%S")
    print("***********************************************************************************************************")
    print("[ {} ] End Global Hub Inspector".format(current))
    print("***********************************************************************************************************")
    print(Style.RESET_ALL)
    
if __name__ == "__main__":
    main()