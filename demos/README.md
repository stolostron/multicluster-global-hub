# Hub-of-Hubs demos

## Prerequisites

1. Three clusters with ACM 2.4 Hubs on them, `hoh` (the _Hub of Hubs_), `hub1` and `hub2` (two Hubs to connect to the Hub of Hubs). The minimal [requirements for ACM 2.4](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/install/installing#requirements-and-recommendations) are sufficient for this demo.
1. Some managed clusters [imported into](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/clusters/managing-your-clusters#importing-a-target-managed-cluster-to-the-hub-cluster) (or [created by](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/clusters/managing-your-clusters#creating-a-cluster)) both `hub1` and `hub2`.
1. [Access to the ACM Web console](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.4/html/web_console/web-console#accessing-your-console) of the three ACM hubs.

## Setup
1.  Define environment variables to hold Kubernetes configurations of the clusters, `TOP_HUB_CONFIG`, `HUB1_CONFIG` and `HUB2_CONFIG`.
1.  Download [demo-magic shell script](https://github.com/paxtonhare/demo-magic) into this directory (_demos_).

    ```
    curl -s https://raw.githubusercontent.com/paxtonhare/demo-magic/master/demo-magic.sh -o demo-magic.sh
    chmod u+x demo-magic.sh
    ```

4.  Follow [the instructions to setup Hub-of-Hubs components](setup).
5.  If you do not have access to `hoh` as the `kubeadmin` user, [add an alternative user as the admin to Hub-of-Hubs RBAC](https://github.com/stolostron/hub-of-hubs-rbac#update-role-bindings-or-role-definitions).

## Demos

1.  [Single Desktop](single_desktop)

    ![Hub-of-Hubs Single Desktop, Cluster view](single_desktop/images/cluster_view.png)

1.  [Multiple Desktops](multiple_desktops)

    ![Hub-of-Hubs Multiple Desktops](multiple_desktops/images/animation.gif)

1.  [Setup](setup)

    Shows deploying Hub-of-Hubs components.

## Tips

* To cancel the prompt message and set it to `$`, run `export PS1=$ `.
* To prevent SSH changing the Terminal title on macOS, run `export PROMPT_COMMAND=""` in the ssh-ed machine.
* To set the Terminal's title on macOS, go to `Shell` -> `Edit title`.
* To set the Terminal's background on macOS, go to `Shell` -> `Edit background`.
* [Working with multiple desktops on macOS](https://www.chriswrites.com/create-multiple-desktops-mission-control/).
* To change the background of a desktop on macOS, right click -> `Change Desktop Background...`.
