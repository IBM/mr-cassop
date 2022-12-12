package eventhandler

import (
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewAnnotationEventHandler() *AnnotationEventHandler {
	return &AnnotationEventHandler{
		EventHandler: &handler.Funcs{},
	}
}

type AnnotationEventHandler struct {
	handler.EventHandler
}

func (h *AnnotationEventHandler) Update(e event.UpdateEvent, ratelimit workqueue.RateLimitingInterface) {
	annotations := e.ObjectNew.GetAnnotations()
	if len(annotations) == 0 {
		return
	}

	ccInstanceName := annotations[v1alpha1.CassandraClusterInstance]
	if ccInstanceName == "" {
		return
	}
	reconcileRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: ccInstanceName, Namespace: e.ObjectNew.GetNamespace()}}
	ratelimit.Add(reconcileRequest)
}

func (h *AnnotationEventHandler) Generic(e event.GenericEvent, ratelimit workqueue.RateLimitingInterface) {
	annotations := e.Object.GetAnnotations()
	if len(annotations) == 0 {
		return
	}

	ccInstanceName := annotations[v1alpha1.CassandraClusterInstance]
	if ccInstanceName == "" {
		return
	}

	reconcileRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: ccInstanceName, Namespace: e.Object.GetNamespace()}}
	ratelimit.Add(reconcileRequest)
}
