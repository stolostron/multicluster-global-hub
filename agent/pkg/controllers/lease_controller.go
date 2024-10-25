package controllers

import (
	"context"
	"os"
	"time"

	"go.uber.org/zap"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var leaseCtrlStarted bool

const (
	leaseUpdateJitterFactor     = 0.25
	defaultLeaseDurationSeconds = 60
)

// leaseUpdater updates lease with given name and namespace in certain period
type leaseUpdater struct {
	log                  *zap.SugaredLogger
	client               client.Client
	leaseName            string
	leaseNamespace       string
	leaseDurationSeconds int32
	healthCheckFuncs     []func() bool
}

// AddLeaseController creates a new LeaseUpdater instance aand add it to given manager
func AddLeaseController(mgr ctrl.Manager, addonNamespace, addonName string) error {
	if leaseCtrlStarted {
		return nil
	}

	var config *rest.Config
	if isAgentTesting, ok := os.LookupEnv("AGENT_TESTING"); ok && isAgentTesting == "true" {
		config = mgr.GetConfig()
	} else {
		var err error
		config, err = rest.InClusterConfig()
		if err != nil {
			return err
		}
	}

	// create new client to create or update lease, the lease is always in the cluster that pod is running
	c, err := client.New(config, client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return err
	}

	// creates the clientset
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	err = mgr.Add(&leaseUpdater{
		log:                  logger.ZapLogger("lease-updater"),
		client:               c,
		leaseName:            addonName,
		leaseNamespace:       addonNamespace,
		leaseDurationSeconds: defaultLeaseDurationSeconds,
		healthCheckFuncs: []func() bool{
			checkAddonPodFunc(logger.ZapLogger("pod-status-checker"),
				kubeClient.CoreV1(),
				addonNamespace,
				"name=multicluster-global-hub-agent"),
		},
	})
	if err != nil {
		return err
	}
	leaseCtrlStarted = true
	return nil
}

// Start starts a goroutine to update lease to implement controller-runtime Runnable interface
func (r *leaseUpdater) Start(ctx context.Context) error {
	wait.JitterUntilWithContext(ctx,
		r.reconcile,
		time.Duration(r.leaseDurationSeconds)*time.Second,
		leaseUpdateJitterFactor,
		true)
	return nil
}

func (r *leaseUpdater) updateLease(ctx context.Context) error {
	lease := &coordinationv1.Lease{}
	err := r.client.Get(ctx, types.NamespacedName{
		Namespace: r.leaseNamespace,
		Name:      r.leaseName,
	}, lease)
	switch {
	case errors.IsNotFound(err):
		// create lease
		newLease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.leaseName,
				Namespace: r.leaseNamespace,
			},
			Spec: coordinationv1.LeaseSpec{
				LeaseDurationSeconds: &r.leaseDurationSeconds,
				RenewTime: &metav1.MicroTime{
					Time: time.Now(),
				},
			},
		}

		// create new lease if it is not found
		if err := r.client.Create(ctx, newLease, &client.CreateOptions{}); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		// update lease
		lease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
		if err := r.client.Update(ctx, lease, &client.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil
}

func (r *leaseUpdater) reconcile(ctx context.Context) {
	r.log.Debug("lease updater is reconciling", "namespace", r.leaseNamespace, "name", r.leaseName)
	for _, f := range r.healthCheckFuncs {
		if !f() {
			// if a healthy check fails, do not update lease.
			return
		}
	}
	// Update lease on managed cluster
	if err := r.updateLease(ctx); err != nil {
		r.log.Error(err, "failed to update lease", "namespace", r.leaseNamespace, "name", r.leaseName)
	}

	r.log.Debug("lease is created or updated", "namespace", r.leaseNamespace, "name", r.leaseName)
}

// checkAddonPodFunc checks whether the agent pod is running
func checkAddonPodFunc(log *zap.SugaredLogger, podGetter corev1client.PodsGetter,
	namespace, labelSelector string,
) func() bool {
	return func() bool {
		pods, err := podGetter.Pods(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			log.Error(err, "failed to get the pod list with label selector",
				"namespace", namespace, "labelSelector", labelSelector)
			return false
		}

		// Ii one of the pods is running, we think the agent is serving.
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				return true
			}
		}

		return false
	}
}
