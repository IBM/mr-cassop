package controllers

import (
	"context"
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

func (r *CassandraClusterReconciler) reconcileCassandraConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	operatorCM, err := r.getConfigMap(ctx, names.OperatorCassandraConfigCM(), r.Cfg.Namespace)
	if err != nil {
		return err
	}

	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ConfigMap(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
	}

	data := util.MergeMap(make(map[string]string), operatorCM.Data)

	if cc.Spec.Cassandra.Persistence.Enabled && cc.Spec.Cassandra.Persistence.CommitLogVolume {
		cassandraYaml := make(map[string]interface{})
		err := yaml.Unmarshal([]byte(data["cassandra.yaml"]), &cassandraYaml)
		if err != nil {
			return errors.Wrap(err, "can't unmarshal 'cassandra.yaml'")
		}

		cassandraYaml["commitlog_directory"] = cassandraCommitLogDir

		cassandraYamlBytes, err := yaml.Marshal(cassandraYaml)
		if err != nil {
			return errors.Wrap(err, "can't marshal 'cassandra.yaml'")
		}

		data["cassandra.yaml"] = string(cassandraYamlBytes)
	}

	desiredCM.Data = data

	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	if err = r.reconcileConfigMap(ctx, desiredCM); err != nil {
		return err
	}

	return nil
}

func cassandraDCConfigVolume(cc *v1alpha1.CassandraCluster, dc v1alpha1.DC) v1.Volume {
	return v1.Volume{
		Name: "config",
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: names.ConfigMap(cc.Name),
				},
				DefaultMode: proto.Int32(v1.SecretVolumeSourceDefaultMode),
			},
		},
	}
}

func cassandraDCConfigVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "config",
		MountPath: "/etc/cassandra-configmaps",
	}
}
