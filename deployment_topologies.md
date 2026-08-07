
## Modes of Deployment

Let us assume that there are n number of ACM hubs deployed (in n number of clusters).

|Mode|Details|Usage|
|-|-|-|
|Standalone Mode|The GH operator can be installed in any cluster.|There is no need from a centralized management of the ACM hubs. We need to have (1) overall dashboard to view Summary status of clusters, policies (2) Near real time and History of key change life cycle events across the fleet.| 
|Default Mode|Ideally the GH operator is installed in n+1th cluster with a ACM hub. Conditionally it may be installed on any of the n hubs if all features are not needed.|There is a need for a centralized management of ACM hub. This is done from the ACM Hub installed with GH. We need to have (1) Global Search (2) overall dashboard to view Summary status of clusters, policies (3) Near real time and History of key change life cycle events across the fleet. (4) Manage the ACM hubs from one single point.|
|Hosted Mode|Ideally the GH operator is installed in n+1th cluster with a ACM hub. Conditionally it may be installed on any of the n hubs if all features are not needed.|There is a need for a limited centralized management of ACM hub. This is done from the ACM Hub installed with GH. We need to have (1) Global Search (2) overall dashboard to view Summary status of clusters, policies (3) Near real time and History of key change life cycle events across the fleet. (4) Limited management the ACM hubs from one single point.|

## Comparitive Features

|Hightlights|Standalone Mode|Default Mode|Hosted Mode|
|-|-|-|-|
|Need ACM at the GH level|No|Yes|Yes|
|How are agents added to the managed hubs|Manually by user using the GH operator.**|Automatically by GH Manager|Automatically by GH Manager|
|Can local cluster be enabled on Managed Hubs|N/A|No|Yes|
|Can all addons be enabled on Managed Hubs|N/A|Yes|clusterproxy, manifestwork and search supported thus far|
|Summary Views at GH dashboard|Yes|Yes|Yes|
|Control Managed Hubs from GH level|N/A|Yes|Yes - but limited due to limited addon support|
|Can we distribute a policy from GH to targeted managed cluster|N/A. They are controlled at individual hubs.|Yes|Unverified/Requires development|
|Can we support Managed Cluster Migration|N/A|Yes|Yes|
|Can we create (BM Clusters) Managed hub from GH|N/A|Yes|No|
|Can we create BM Clusters on Managed hub from GH|N/A|Yes|No|
|Can we supported Hosted Clusters in Managed hub from GH|Yes, Managed hub may have hosted clusters, but they are not managed from GH|No. Deploying hosted clusters need local-cluster enablement|Yes|
|Future evaluation|enable acm hub control by using user creds if Direct Authentication is enabled|see if we can work with local-cluster enabled|enable support of other addons|

** When certs for Kafka is rotated in the GH cluster, the agents need to be given those new certs. This is not managed by the GH today.
