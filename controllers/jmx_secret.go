package controllers

import (
	"context"
	"fmt"
	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
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

func (r *CassandraClusterReconciler) reconcileJMXSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	desiredSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.JMXRemoteSecret(cc),
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
			Namespace: cc.Namespace,
		},
		Type: v1.SecretTypeOpaque,
	}

	data := make(map[string][]byte)
	data["jmxremote.password"] = []byte(fmt.Sprintf("%s %s", cc.Spec.Cassandra.Auth.User, cc.Spec.Cassandra.Auth.Password))
	data["jmxremote.access"] = []byte(fmt.Sprintf(`%s readwrite
  create javax.management.monitor.*, javax.management.timer.*
  unregister`, cc.Spec.Cassandra.Auth.User))

	desiredSecret.Data = data

	if err := controllerutil.SetControllerReference(cc, desiredSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredSecret.Name, Namespace: desiredSecret.Namespace}, actualSecret)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating jmx secret")
		if err = r.Create(ctx, desiredSecret); err != nil {
			return errors.Wrap(err, "Unable to create jmx secret")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get jmx secret")
	} else if !compare.EqualSecret(actualSecret, desiredSecret) {
		r.Log.Info("Updating jmx secret")
		r.Log.Debug(compare.DiffSecret(actualSecret, desiredSecret))
		actualSecret.Labels = desiredSecret.Labels
		actualSecret.Type = desiredSecret.Type
		actualSecret.Data = desiredSecret.Data
		if err = r.Update(ctx, actualSecret); err != nil {
			return errors.Wrap(err, "Could not Update jmx secret")
		}
	} else {
		r.Log.Debug("No updates for jmx secret")

	}

	return nil
}

func jmxSecretVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	return v1.Volume{
		Name: "jmxremote",
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName:  names.JMXRemoteSecret(cc),
				DefaultMode: proto.Int32(0400),
			},
		},
	}
}

func jmxSecretVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "jmxremote",
		MountPath: "/etc/cassandra-jmxremote",
	}
}
