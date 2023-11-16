# Scenario 1: 2 hubs, 1000, 2000, 3000 clusters

## Simultation Steps

0. Start counter with `./doc/simulation/inspector/cmd/counter.sh start`

1. Install the global hub and then join the 2 simulated managed hubs, each with 1000 clusters, into it
   ```bash
   # simulate 2 hubs and each with 1000 clusters
   ./doc/simulation/setup/setup-cluster.sh 2 1000 
   # deploy agent 
   kubectl label mcl hub1 vendor=OpenShift --overwrite
   kubectl label mcl hub2 vendor=OpenShift --overwrite
   ```

2. Detach the 2 hubs, and then simulate 2000 clusters on each hub, rejoin them into global hub
    ```bash
    # simulate 2 hubs and each with 2000 clusters
    ./doc/simulation/setup/setup-cluster.sh 2 2000 1001
    # deploy agent 
    kubectl label mcl hub1 vendor=OpenShift --overwrite
    kubectl label mcl hub2 vendor=OpenShift --overwrite
    ```

3. Detach the 2 hubs, and then simulate 3000 clusters on each hub, rejoin them into global hub
    ```bash
    # simulate 2 hubs and each with 3000 clusters
    ./doc/simulation/setup/setup-cluster.sh 2 3000 2001
    # deploy agent 
    kubectl label mcl hub1 vendor=OpenShift --overwrite
    kubectl label mcl hub2 vendor=OpenShift --overwrite
    ```

4. Stop counter and draw the data trend in database 

    ```bash
    # draw the graph
    ./doc/simulation/inspector/cmd/counter.sh draw
    # stop the counter
    ./doc/simulation/inspector/cmd/counter.sh start
    ```

5. Generate the CPU and Memory of the Componenens
   
   ```bash
   ./doc/simulation/inspector/cmd/check.sh "2023-11-15 08:40:00" "2023-11-15 09:56:37"
   ```


## The Count of the Global Hub Data from database

- The Managed Clusters

![Manager Cluster](./images/1-count-initialization.png)


### The CPU and Memory Consumption of the Global Hub Components

- Multicluster Global Hub Manager
![Global Hub Manager CPU](./images/1-manager-cpu-usage.png)
![Global Hub Manager Memory](./images/1-manager-memory-usage.png)

- Multicluster Global Hub Grafana
![Global Hub Grafana CPU](./images/1-grafana-cpu-usage.png)
![Global Hub Grafana Memory](./images/1-manager-memory-usage.png)

- Multicluster Global Hub Operator
![Global Hub Operator CPU](./images/1-operator-cpu-usage.png)
![Global Hub Operator Memory](./images/1-operator-memory-usage.png)

### The CPU and Memory Consumption of the Middlewares

- Multicluster Global Hub Kafka Broker
![Global Hub Kafka Broker CPU](./images/1-kafka-broker-cpu-usage.png)
![Global Hub Kafka Broker Memory](./images/1-kafka-broker-memory-usage.png)

- Multicluster Global Hub Kafka Zookeeper
![Global Hub Kafka Zookeeper CPU](./images/1-kafka-zookeeper-cpu-usage.png)
![Global Hub Kafka Zookeeper Memory](./images/1-kafka-zookeeper-memory-usage.png)

- Multicluster Global Hub Postgres
![Global Hub Postgres CPU](./images/1-postgres-cpu-usage.png)
![Global Hub Postgres Memory](./images/1-postgres-memory-usage.png)