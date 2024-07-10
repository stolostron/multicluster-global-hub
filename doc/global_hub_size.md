# Global Hub Size

The global hub size includes the main components of the global hub: operator, manager, grafana, agent, built-in kafka, and postgres. The results listed below are based on the [simulations](./simulation/scenario) conducted by [the global hub inspector](./simulation/inspector).

## Release 2.11(Global Hub 1.2)

- Scale: 10 managed hubs, each with 300 managed clusters, 50 policies

- CPU and Memory Summary

  ---
  | Type               | Manager | Agent | Operator | Grafana | Postgres | Kafka Broker | Kafka Zookeeper |
  |---                 |---      |---    |---       |---      |---       |---           |---              |
  | Request CPU(m)     | 5       | 10    | 2        | 5       | 100      | 20           | 10              |
  | Limit CPU(m)       | 500     | 50    | 100      | 50      | 8000     | 200          | 50              |
  | Request Memory(Mi) | 60      | 300   | 70       | 150     | 60       | 1.5 Gi       | 800             |
  | Limit Memory(Mi)   | 500     | 1200  | 200      | 800     | 1000     | 5   Gi       | 2   Gi          | 

- Reference: [Scenario 3: manage 3,000 clusters and 500 policies](./simulation/scenario/Scenario3:%203000_clusters_500_policies.md)