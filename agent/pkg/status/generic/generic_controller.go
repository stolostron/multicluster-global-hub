package generic

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/interfaces"
)

var _ interfaces.Controller = &genericController{}

type genericController struct {
	instance  func() client.Object
	predicate predicate.Predicate
}

func NewGenericController(instance func() client.Object, predicate predicate.Predicate) interfaces.Controller {
	return &genericController{
		instance:  instance,
		predicate: predicate,
	}
}

func (g *genericController) Instance() client.Object {
	return g.instance()
}

func (g *genericController) Predicate() predicate.Predicate {
	return g.predicate
}
