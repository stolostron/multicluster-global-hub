# How does Global Hub Work
The Multicluster Global Hub [architecture](./README.md) contains two key sets of microservices that create the high level summary of the many policies deployed across many clusters:

- Multicluster Global Hub manager
- Multicluster Global Hub agent

## Scenario
The following sections guide you through a possible installation scenario.
1. There are 700 managed clusters managed by multiple Red Hat Advanced Cluster Management for Kubernetes hub clusters.
2. Each of these managed clusters have 30 policies deployed on them.
3. There are a total of 100 policies.
4. A Multicluster Global Hub is added to deliver these [Use Cases](./global_hub_use_cases.md).

### Summarization Process
You can summarize a single line that shows policy compliance across time. In this scenario, there are 21,000 (700*30) cluster policy statuses that need to be summarized. 

#### Key Steps
1. Events for the following policies [flow](#dataflow) into Multicluster Global Hub:
    - Policy creation
    - Policy propapagtion to managed clusters 
    - Policy compliance for each of the managed cluster 
1. The raw events are saved in the database.
1. The current status of each policy is also saved in the database.
1. In this example, a summarization routine starts on the cluster that hosts the Multicluster Global Hub each night at midnight. It summarizes a policy running on a cluster as `compliant`, `non-compliant`, or `pending` for the previous day. This summary is available for all policies. This summary is based on the following input:
    - Compliance state of the `policy on the cluster` at *end of the previous day*
    - Events on the `policy on the cluster` *during the previous day*
    - Compliance state of a cluster is a logical `AND` of daily summarized status of all polcies on the cluster.

#### Summarization Rule
|State of Policy on a cluster at end of previous day|Events related to the Policy on the cluster during the previous day| Calculated Summarized state for the previous day|
|---|---|---|
|compliant| No noncompliant events occurred during the day| compliant|
|compliant| One or more noncompliant events occurred during the day| non-compliant|
|non-compliant| does not matter| non-compliant|
|pending| does not matter| pending|

In the long run, the desired state is full compliance. Daily variations as captured in the previous table need to be investigated. This investigation also raises any unexpected behaviour. For example, the fleet is largely compliant, except for a few outliers.

#### Dataflow    
![DataflowDiagram](architecture/mcgh-data-flow.png)

- Multicluster Global Hub operator controls the life cycle of the Multicluster Global Hub manager and global hub agent.
- Kafka and the database can run on the Global cluster or outside of it.
- The Red Hat Advanced Cluster Management hub cluster also runs on the Multicluster Global Hub cluster, but does not participate in the daily functioning of the global hub.

**Notes:**
- Multicluster Global Hub Operator controls the life cycle of the multicluster global hub manager and global hub agent.
- Kafka and the database can run on the Global cluster or outside of it.
- Red Hat Advanced Cluster Management hub also runs on the Multicluster Global Hub cluster, but does not participate in the daily functioning of the global hub.

### Running the Summarization Process manually

Manually running the summarization process typically means we need to recover history compliances from the day without initialization. The simplest way is to use the previous day's history as the initial state for the recover day's history.

#### Procedure steps

1.Connect to the database.

  You can use clients such as pgAdmin, tablePlush, etc. to connect to the Global Hub database. Alternatively, use the following command to connect directly.

  ```bash
  kubectl exec -it multicluster-global-hub-postgresql-0 -n multicluster-global-hub -- psql -U postgres -d hoh
  ```

2.Determine the date when it needs to be run, such as `2023-07-07`.

  Finding the local compliance job failure information from the dashboard metrics or table `history.local_compliance_job_log`. In this case, if it's `2023-07-07`. That means there was a failure while initializing the state for the day of July 7, 2023.

3.Recover the initial compliance of `2023-07-07` by running the following SQL:

  ```SQL
  -- call the func to generate the initial data of '2023-07-07' by inheriting '2023-07-06'
  CALL history.generate_local_compliance('2024-07-07');
  ```
