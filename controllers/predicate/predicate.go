package predicate

import (
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = &cassandraClusterReconcilePredicate{}

type cassandraClusterReconcilePredicate struct {
	logger *zap.SugaredLogger
}

func NewPredicate(logr *zap.SugaredLogger) predicate.Predicate {
	if logr == nil {
		logr = zap.NewNop().Sugar()
	}
	return &cassandraClusterReconcilePredicate{
		logger: logr,
	}
}

func (p *cassandraClusterReconcilePredicate) Create(e event.CreateEvent) bool {
	p.logger.Debugf("Create event for resource %T: %s/%s", e.Object, e.Meta.GetNamespace(), e.Meta.GetName())
	return true
}

func (p *cassandraClusterReconcilePredicate) Delete(e event.DeleteEvent) bool {
	p.logger.Debugf("Delete event for resource %T: %s/%s", e.Object, e.Meta.GetNamespace(), e.Meta.GetName())
	return true
}

func (p *cassandraClusterReconcilePredicate) Update(e event.UpdateEvent) bool {
	p.logger.Debugf("Update event for resource %T: %s/%s", e.ObjectNew, e.MetaNew.GetNamespace(), e.MetaNew.GetName())
	return true
}

func (p *cassandraClusterReconcilePredicate) Generic(e event.GenericEvent) bool {
	p.logger.Debugf("Generic event for resource %T: %s/%s", e.Object, e.Meta.GetNamespace(), e.Meta.GetName())
	return true
}
