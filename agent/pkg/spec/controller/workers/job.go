package workers

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sJobHandlerFunc is a function for running a k8s job by a k8s worker.
// type K8sJobHandlerFunc func(context.Context, client.Client, interface{})
// K8sJob represents the job to be run by a k8sWorker from the pool.
type Job struct {
	obj         interface{}
	handlerFunc func(context.Context, client.Client, interface{})
}

// NewJob creates a new instance of K8sJob.
func NewJob(obj interface{}, handlerFunc func(context.Context, client.Client, interface{})) *Job {
	return &Job{
		obj:         obj,
		handlerFunc: handlerFunc,
	}
}
