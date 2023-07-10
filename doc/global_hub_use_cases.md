# Goal of Global Hub

## Primary Use Cases
As enterprises evolve to have many Red Hat Advanced Cluster Management for Kubernetes hubs to manage their fleet, there is a need to be able to look at some subset of data across the fleet on a single pane of glass. The Global Hub View provides this information. We start with Global View of Policies as the first step of building Global Views.

### Use Case 1

| Elements  | Description  |
|---|---|
|Precondition|Red Hat Advanced Cluster Management policies have been set up with correctly defined standards, categories and controls. Red Hat Advanced Cluster Management policies have been distributed to clusters that are being managed by different Red Hat Advanced Cluster Management hubs using gitops. The compliance results from groups of Red Hat Advanced Cluster Management policies are aggregated for daily review.|
|Trigger |Need to provide a report for the last 30 days on corporate controls in place for production clusters. |
|Success Flow (nothing goes wrong)|Shows the count of compliance states for the last 30 days. In a successful scenario, these compliance states are all trending up across time. This is one line that represents a group of policies. New clusters that are imported might indicate high levels of initial compliance drift which should indicate the decreasing trend over time. An entire new hub region could be brought ‘online’, and we would expect the compliance drift to decrease as policy controls are enforced.|
|Alternative Flows (something has gone wrong) |A security auditor needs additional details about a specific noncompliance event. The ad-hoc query requires drill down into a local-hub which is outside of the global hub itself.|


### Use Case 2

|Elements|Description|
|---|---|
|Precondition|Red Hat Advanced Cluster Management policies have been set up with correctly defined standards, categories and controls. Red Hat Advanced Cluster Management policies have been rolled out to clusters that are being managed by different Red Hat Advanced Cluster Management hubs using gitops. The cluster groups  are summarized at a daily level.|
|Trigger |Need a report for the last 30 days of compliance for production clusters against all policies |
|Success Flow (nothing goes wrong)|Shows the count of clusters in production group with any vulnerabilities in the last 30 days. We can filter this data by using different vulnerabilities, if needed. In a best case scenario, they are all trending down over time.  This is one line that represents a group of clusters.|

### Using Global Hub
1. There are more than one Red Hat Advanced Cluster Management hubs (Red Hat Advanced Cluster Management version 2.7 or higher) in the problem domain.
1. Install `another` Red Hat Advanced Cluster Management 2.7 hub on another cluster and install the Multicluster Global Hub (MCGH) operator on the same cluster. And create the MCGH custom resource.
1. After disabling `self-management` in the pre-existing Red Hat Advanced Cluster Management hubs, import them as managed clusters in the newly created Red Hat Advanced Cluster Management hub.
1. The policy data from the various Red Hat Advanced Cluster Management hubs start to flow into the MCGH, and summary views start to populate after midnight in the timezone where the cluster on which MCGH is installed.

## Non Goals
1. Control the lifecycle of the existing Red Hat Advanced Cluster Management hubs that report to it.
    - This can be handled by the Red Hat Advanced Cluster Management hub that is co-located on the Global Hub cluster that imports the existing Red Hat Advanced Cluster Management hubs as managed clusters. The Global Hub microservices are not part of this process.
1. Push Policies or Applications to the existing Red Hat Advanced Cluster Management hubs from the Global Hub.
    - This can be handled by regular gitops support of Red Hat Advanced Cluster Management.

## Relation with Red Hat Advanced Cluster Management
1. Global Hub uses the Red Hat Advanced Cluster Management `ManifestWork` API to propagate the `multicluster global hub agent` to the Red Hat Advanced Cluster Management hub. 
1. The `multicluster global hub agent` listens to activity on the Red Hat Advanced Cluster Management hub by listening to the Kube API Server and sends relevant events to the Messaging subsystem. The messaging subsystem is Kafka. You can use this subsystem to listen to other activities and propagate events to the messaging subsystem.  
1. The microservices that run on the Global Hub side can be installed on any cluster. Because it uses the Red Hat Advanced Cluster Management `Manifestwork` API, they are colocated for convenience.  
