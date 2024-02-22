package generic

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ Controller = &genericController{}

type genericController struct {
	instance  func() client.Object
	predicate predicate.Predicate
}

func NewGenericController(instance func() client.Object, predicate predicate.Predicate) Controller {
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
