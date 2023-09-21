# Global Hub Inspector

The inspector is inspired by [acm-inspector](https://github.com/bjoydeep/acm-inspector). It aims to easily obtain the operating status of the global hub and determine the health of the system, so as to support exploring the scalability.

## Prerequisites

1. Set the `KUBECONFIG` so that you can connect the OCP.
2. Expose the postgres endpoint to allow get the database connection. You can achieve this by running `kubectl patch postgrescluster postgres -p '{"spec":{"service":{"type":"LoadBalancer"}}}'  --type merge -n multicluster-global-hub`.
3. The `python3` and the tool `pip3` have been installed on your environment.
4. Enable the `Prometheus` on your global hub.
5. Running the `pip3 install -r ./doc/simulation/inspector/requirements.txt` to install dependencies.

## Running the inspector

### Count the records of database
   
  - Start a backend process to count the records
    
    The statistical data includes:

    1. The count of the managed clusters from all the hubs
    2. The count of the events from Replicas policies
    3. The count of the compliances from all the replicas polices
    3. The count of the compliant and non-compliant polices when rotating the policies status
  
    ```bash
    # override the previous csv file
    ./doc/simulation/inspector/cmd/counter.sh start
    # append the count result to the previous files
    ./doc/simulation/inspector/cmd/counter.sh continue
    ```
  
  - Draw the count results [ Optional: The picture also generate in the next step ]

    ```bash
    ./doc/simulation/inspector/cmd/counter.sh draw
    ```
  
  - Stop the backend process
 
    ```bash
    ./doc/simulation/inspector/cmd/counter.sh stop
    ```
### Get CPU and Memory information

  ```bash
  # The time range is from seven days ago to the current time
  ./doc/simulation/inspector/cmd/check.sh 

  # The time range from the "2023-09-18 00:00:00" to the current time
  ./doc/simulation/inspector/cmd/check.sh "2023-09-18 00:00:00"

   # The time range from the "2023-09-18 00:00:00" to the "2023-09-20 00:00:00"
  ./doc/simulation/inspector/cmd/check.sh "2023-09-18 00:00:00" "2023-09-20 00:00:00"
  ```

All the csv file and picture will be save on the folder `doc/simulation/inspector/output`


## Note

This has been tested using Python 3.11.4 on OCP 4.12.18.