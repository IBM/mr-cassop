package cassandrarestore

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/icarus"
	"github.com/ibm/cassandra-operator/controllers/names"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// CassandraRestoreReconciler reconciles a CassandraRestore object
type CassandraRestoreReconciler struct {
	client.Client
	Log          *zap.SugaredLogger
	Scheme       *runtime.Scheme
	Cfg          config.Config
	Events       *events.EventRecorder
	IcarusClient func(coordinatorPodURL string) icarus.Icarus
}

// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandrarestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandrarestores/status,verbs=get;update;patch

func (r *CassandraRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cr := &v1alpha1.CassandraRestore{}
	err := r.Get(ctx, req.NamespacedName, cr)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if cr.Status.State == icarus.StateCompleted {
		r.Log.Debugf("Restore %s is completed", cr.Name)
		return ctrl.Result{}, nil
	}

	cc := &v1alpha1.CassandraCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: cr.Spec.CassandraCluster, Namespace: cr.Namespace}, cc)
	if err != nil {
		if kerrors.IsNotFound(err) {
			errMsg := fmt.Sprintf("Restore failed. CassandraCluster %s not found", cr.Spec.CassandraCluster)
			r.Log.Warn(errMsg)
			r.Events.Warning(cr, events.EventCassandraClusterNotFound, errMsg)
			return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
		}
		return ctrl.Result{}, err
	}

	if !cc.Status.Ready {
		r.Log.Warnf("CassandraCluster %s/%s is not ready. Not starting backup, trying again in %s...", cc.Namespace, cc.Name, r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	cb := &v1alpha1.CassandraBackup{}
	if len(cr.Spec.CassandraBackup) > 0 {
		err = r.Get(ctx, types.NamespacedName{Name: cr.Spec.CassandraBackup, Namespace: cr.Namespace}, cb)
		if err != nil {
			if kerrors.IsNotFound(err) {
				errMsg := fmt.Sprintf("Restore failed. CassandraBackup %s not found", cr.Spec.CassandraBackup)
				r.Log.Warn(errMsg)
				r.Events.Warning(cr, events.EventCassandraBackupNotFound, errMsg)
				return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
			}
			return ctrl.Result{}, err
		}
	}

	secretName := cr.Spec.SecretName
	if len(secretName) == 0 {
		secretName = cb.Spec.SecretName
	}

	storageCredentials := &v1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cr.Namespace}, storageCredentials)
	if err != nil {
		if kerrors.IsNotFound(err) {
			errMsg := fmt.Sprintf("Failed to create backup for cluster %q. Storage credentials secret %q not found.", cb.Spec.CassandraCluster, cb.Spec.SecretName)
			r.Log.Warn(errMsg)
			r.Events.Warning(cb, events.EventStorageCredentialsSecretNotFound, errMsg)
			return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
		}

		return ctrl.Result{}, err
	}

	err = v1alpha1.ValidateStorageSecret(r.Log, storageCredentials, cb.StorageProvider())
	if err != nil {
		errMsg := fmt.Sprintf("Storage credentials secret %q is invalid: %s", cb.Spec.SecretName, err.Error())
		r.Log.Warn(errMsg)
		r.Events.Warning(cb, events.EventStorageCredentialsSecretNotFound, errMsg)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	dcName := cc.Spec.DCs[0].Name
	svc := names.DC(cc.Name, dcName)
	//always use the same pod as the coordinator as only that pod has the global request info
	coordinatorPodURL := fmt.Sprintf("http://%s-0.%s.%s.svc.cluster.local:%d", svc, svc, cc.Namespace, v1alpha1.IcarusPort)

	ic := r.IcarusClient(coordinatorPodURL)

	res, err := r.reconcileRestore(ctx, ic, cr, cb, cc)
	if err != nil {
		if statusErr, ok := errors.Cause(err).(*kerrors.StatusError); ok && statusErr.ErrStatus.Reason == metav1.StatusReasonConflict {
			r.Log.Info("Conflict occurred. Retrying...", zap.Error(err))
			return ctrl.Result{Requeue: true}, nil //retry but do not treat conflicts as errors
		}

		r.Log.Errorf("%+v", err)
		return ctrl.Result{}, err
	}

	return res, nil
}

func SetupCassandraRestoreReconciler(r reconcile.Reconciler, mgr manager.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		Named("cassandrarestore").
		For(&v1alpha1.CassandraRestore{})

	return builder.Complete(r)
}
