package controllers

import (
	"context"
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcilePrometheusConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	operatorCM, err := r.getConfigMap(ctx, names.OperatorPrometheusConfigCM(), r.Cfg.Namespace)
	if err != nil {
		return err
	}
	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.PrometheusConfigMap(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
		Data: operatorCM.Data,
	}
	if err = controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	if err = r.reconcileConfigMap(ctx, desiredCM); err != nil {
		return err
	}
	return nil
}

func prometheusVolume(cc *v1alpha1.CassandraCluster) v1.Volume {
	return v1.Volume{
		Name: "prometheus-config",
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: names.PrometheusConfigMap(cc.Name),
				},
				DefaultMode: proto.Int32(v1.ConfigMapVolumeSourceDefaultMode),
			},
		},
	}
}

func prometheusVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "prometheus-config",
		MountPath: "/prometheus/prometheus.yaml",
		SubPath: "prometheus.yaml",
	}
}
