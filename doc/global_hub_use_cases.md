# Goal of Global Hub

## Primary Use Cases
As enterprises evolve to have many ACM hubs to manage their fleet, there is a need to be able to look at some subset of data across the fleet from a single pane of glass. This is where the Global Hub View comes in. We start with Global View of Policies as first step of building Global Views.

### Use Case 1
|||
|---|---|
|Precondition|ACM policies have been set up with correctly defined standards, categories and controls ; ACM policies have been distributed to clusters that are being managed by different ACM hubs using gitops; The compliance results from groups of ACM policies are aggregated for daily review|
|||
|Trigger |Need to provide a report for the last 30 days on corporate controls in place for Production clusters. |
|||
|Success Flow (nothing goes wrong)|Shows the count of compliance states for the last 30 days.  In a successful scenario these compliance states are all trending up across time. (this is one line that represents a group of policies). New clusters that are imported might indicate high levels of initial compliance drift which should indicate the decreasing trend over time. An entire new hub region could be brought ‘online’ and again we would expect the compliance drift decreasing as policy controls are enforced.|
|||
|Alternative Flows (something has gone wrong) |A security auditor needs additional details around a specific noncompliance event. The ad-hoc query requires drill down into a local-hub which is outside of the global hub itself.|
|||


### Use Case 2
|||
|---|---|
|Precondition|ACM policies have been set up with correctly defined standards, categories and controls ; ACM policies have been rolled out to clusters that are being managed by different ACM hubs using gitops; The cluster groups  are summed up to a daily level.|
|||
|Trigger |Need a report for the last 30 days of compliance for Production clusters against all policies |
|||
|Success Flow (nothing goes wrong)|Shows the count of clusters -which are in production group -  with any vulnerabilities for the last 30 days.  We can filter this data by different vulnerabilities if needed. In a happy scenario they are all trending down across time.  (this is one line that represents a group of clusters).|
|||

### Using Global Hub
1. There are more than one ACM Hub (ACM 2.7 or higher) in the problem domain.
1. Install `another` ACM 2.7 Hub on another cluster and install the multicluster global hub (MCGH) operator on the same cluster. And create the MCGH Custom Resource.
1. After disabling `self-management` in the pre existing ACM Hubs, import them as managed cluster in the newly created ACM hub.
1. The policy data from the various ACM Hubs will start to flow into the MCGH and summary views will start to get populated after midnight (timezone of the cluster in which MCGH is installed )

## Non Goals
1. Control the life cycle of the existing ACM hubs that 
report to it.
    - This can be handled by the ACM hub collocated on the Global Hub cluster which imports the existing ACM hubs as managed clusters. The Global Hub microservices do not participate in this in any shape or form
1. Push Policies or Applications to the existing ACM hubs from the Global Hub.
    - This can be handled by regular gitops support of ACM.

## Relation with ACM
1. Global Hub uses ACM API (ManifestWork) to propagate the `multicluster global hub agent` to the ACM Hub. 
1. The `multicluster global hub agent` listens to activity on the ACM hub (by listening to the Kube API Server) and sends relevant events to the Messaging sub-system - which is Kafka. This could be adopted to listen to other activities and propagate events to the messaging sub-system.  
1. The microservices that run on the Global Hub side can be installed on any cluster. However, since it uses the ACM Manifestwork API, they are collocated just for convenience.  
