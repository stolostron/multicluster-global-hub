# Scenario 1: 2 hubs, 1000, 2000, 3000 clusters

## Simultation Steps

0. Start counter - `./doc/simulation/inspector/cmd/counter.sh start`

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

![Manager Cluster](./../../images/global-hub-count-cluster.png)


### The CPU and Memory Consumption of the Global Hub Components

- Multicluster Global Hub Manager
![Global Hub Manager CPU](./../images/global-hub-manager-cpu-usage.png)
![Global Hub Manager Memory](./../images/global-hub-manager-memory-usage.png)

  | Manager | Request | Limit |
  |---|---|---|
  | CPU | 0.1 | 0.5 |
  | Memory | 100 Mi | 300 Mi |

- Multicluster Global Hub Grafana
![Global Hub Grafana CPU](./../images/global-hub-grafana-cpu-usage.png)
![Global Hub Grafana Memory](./../images/global-hub-grafana-memory-usage.png)

- Multicluster Global Hub Operator
![Global Hub Operator CPU](./../images/global-hub-operator-cpu-usage.png)
![Global Hub Operator Memory](./../images/global-hub-operator-memory-usage.png)

  | Operator | Request | Limit |
  |---|---|---|
  | CPU | 0.001 | 0.05 |
  | Memory | 100 Mi | 200 Mi |

- Multicluster Global Hub Agent on Managed Hub
![Global Hub Agent CPU](./../images/global-hub-agent-cpu-usage.png)
![Global Hub Agent Memory](./../images/global-hub-agent-memory-usage.png)

  | Agent | Request | Limit |
  |---|---|---|
  | CPU | 0.01 | 0.05 |
  | Memory | 200 Mi | 1000 Mi |

### The CPU and Memory Consumption of the Middlewares

- Multicluster Global Hub Kafka Broker
![Global Hub Kafka Broker CPU](./../images/global-hub-kafka-broker-cpu-usage.png)
![Global Hub Kafka Broker Memory](./../images/global-hub-kafka-broker-memory-usage.png)
  | KafkaBroker | Request | Limit |
  |---|---|---|
  | CPU | 0.004 | 0.2 |
  | Memory | 1.5 Gi | 5 Gi |

- Multicluster Global Hub Kafka Zookeeper
![Global Hub Kafka Zookeeper CPU](./../images/global-hub-kafka-zookeeper-cpu-usage.png)
![Global Hub Kafka Zookeeper Memory](./../images/global-hub-kafka-zookeeper-memory-usage.png)

  | KafkaZookeeper | Request | Limit |
  |---|---|---|
  | CPU | 0.01 | 0.02 |
  | Memory | 0.8 Gi | 2 Gi |

- Multicluster Global Hub Postgres
![Global Hub Postgres CPU](./../images/global-hub-postgres-cpu-usage.png)
![Global Hub Postgres Memory](./../images/global-hub-postgres-memory-usage.png)

  | Postgres | Request | Limit |
  |---|---|---|
  | CPU | 0.5 | 5 |
  | Memory | 128Mi | 4Gi |

## Related Material

- [acm-inspector](https://github.com/bjoydeep/acm-inspector)
- [Red Hat Advanced Cluster Management Hub-of-hubs Scale and Performance Tests](https://docs.google.com/presentation/d/1z6hESoacKRHuBQ-7I8nqWBuMnw7Z6CAw/edit#slide=id.p1)
- [Replace Global Hub Transport with Cloudevents](https://github.com/stolostron/multicluster-global-hub/issues/310)