package controllers

import (
	"context"
	"github.com/gogo/protobuf/proto"
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

func (r *CassandraClusterReconciler) reconcileScriptsConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	//deployed with the operator
	operatorCM := &v1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: names.OperatorScriptsCM(), Namespace: r.Cfg.Namespace}, operatorCM); err != nil {
		if kerrors.IsNotFound(err) {
			return errors.Wrap(err, "operator scripts configmap doesn't exist")
		}
		return errors.Wrap(err, "can't get operator scripts configmap")
	}

	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ScriptsConfigMap(cc),
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
			Namespace: cc.Namespace,
		},
		Data: operatorCM.Data,
	}

	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualCM := &v1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, actualCM)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating scripts ConfigMap")
		if err = r.Create(ctx, desiredCM); err != nil {
			return errors.Wrap(err, "Unable to create scripts ConfigMap")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get scripts ConfigMap")
	} else if !compare.EqualConfigMap(actualCM, desiredCM) {
		r.Log.Info("Updating scripts ConfigMap")
		r.Log.Debug(compare.DiffConfigMap(actualCM, desiredCM))
		actualCM.Labels = desiredCM.Labels
		actualCM.Data = desiredCM.Data
		if err = r.Update(ctx, actualCM); err != nil {
			return errors.Wrap(err, "Could not Update scripts ConfigMap")
		}
	} else {
		r.Log.Debug("No updates for scripts ConfigMap")

	}
	return nil
}

func scriptsVolume(cc *v1alpha1.CassandraCluster) v1.Volume {
	return v1.Volume{
		Name: "scripts-config",
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: names.ScriptsConfigMap(cc),
				},
				DefaultMode: proto.Int32(448),
			},
		},
	}
}

func scriptsVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "scripts-config",
		MountPath: "/scripts",
	}
}
