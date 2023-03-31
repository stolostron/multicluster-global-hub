package hubofhubs

import (
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// this controller is responsible for updating the status of the global hub mgh cr
type GlobalHubStatusReconciler struct {
	manager.Manager
	client.Client
	KubeClient     kubernetes.Interface
	Scheme         *runtime.Scheme
	LeaderElection *commonobjects.LeaderElectionConfig
}
