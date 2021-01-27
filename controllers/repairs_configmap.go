package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileRepairsConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.RepairsConfigMap(cc.Name),
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentReaper),
			Namespace: cc.Namespace,
		},
	}
	repairs, err := generateRepairs(cc)
	if err != nil {
		return errors.Wrap(err, "Could not parse repairs list")
	}
	desiredCM.Data = repairs
	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualCM := &v1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, actualCM)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating reaper repairs ConfigMap")
		if err = r.Create(ctx, desiredCM); err != nil {
			return errors.Wrap(err, "Unable to create reaper repairs ConfigMap")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get reaper repairs ConfigMap")
	} else if !compare.EqualConfigMap(actualCM, desiredCM) {
		r.Log.Info("Updating reaper repairs ConfigMap")
		r.Log.Debug(compare.DiffConfigMap(actualCM, desiredCM))
		actualCM.Labels = desiredCM.Labels
		actualCM.Data = desiredCM.Data
		if err = r.Update(ctx, actualCM); err != nil {
			return errors.Wrap(err, "Could not Update reaper repairs ConfigMap")
		}
	} else {
		r.Log.Debug("No updates for reaper repairs ConfigMap")
	}

	return nil
}

func generateRepairs(cc *v1alpha1.CassandraCluster) (map[string]string, error) {
	repairs := make(map[string]string)
	for i, r := range cc.Spec.Reaper.ScheduleRepairs.Repairs {
		var data []byte
		data, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		repairs[fmt.Sprint(i)] = fmt.Sprint(data)
	}
	return repairs, nil
}
