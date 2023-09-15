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

The manual summarization process consists of two subtasks:
- Insert the cluster policy data of that day from [Materialized View](https://www.postgresql.org/docs/current/rules-materializedviews.html)  `local_compliance_view_<yyyy_MM_dd>` to `history.local_compliance`.
- Update the `compliance` and policy flip `frequency` of that day to `history.local_compliance` based on `event.local_policies`.

#### Procedure steps

1. Connect to the database.
   
   You can use clients such as pgAdmin, tablePlush, etc. to connect to the Global Hub database to execute the SQL statements involved in the next few steps. If your postgres database is installed through [this script](../operator/config/samples/storage/deploy_postgres.sh), you can directly connect to the database on the cluster by running the following command:
   ```
   kubectl exec -t $(kubectl get pods -n multicluster-global-hub -l postgres-operator.crunchydata.com/role=master -o jsonpath='{.items..metadata.name}') -n multicluster-global-hub -c database -- psql -d hoh
   ```
       
2. Determine the date when it needs to be run, such as `2023-07-06`.

    If you find that there is no compliance information on the dashboard for `2023-07-06`, then find the the job failure information of the day following this day in the `history.local_compliance_job_log`. In this case, it is `2023-07-07`. It can be determined that `2023-07-06` is the date when we need to manually run the summary processes.

3. Check whether the Materialized View of `history.local_compliance_view_2023_07_06` exists by running the following command:
    ```
    select * from history.local_compliance_view_2023_07_06;
    ```
    - If the view exists, load the view records to `history.local_compliance`
      ```sql
      -- exec the insert func for that day '2023_07_06'
      SELECT history.insert_local_compliance_job('2023_07_06');
      ```

    - If the view not exists, inherit the history compliance records of the day before that day, that is `2023_07_05`
      ```sql
      -- call the func to generate the data of '2023_07_06' by inheriting '2023_07_05'
      CALL history.inherit_local_compliance_job('2023_07_05', '2023_07_06');
      ```

4. Update the `compliance` and `frequency` information of that day to `history.local_compliance`
    ```sql
    -- call the func to update records start with '2023-07-06', end with '2023-07-07'
    SELECT history.update_local_compliance_job('2023_07_06', '2023_07_07');
    ```
5. Find the records of that day generated in `history.local_compliance`. You can safely delete the Materialized View `history.local_compliance_view_2023_07_06` by running the following command:
    ```
    DROP MATERIALIZED VIEW IF EXISTS history.local_compliance_view_2023_07_06;
    ```