package controllers

import (
	"context"
	"fmt"
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
	"sigs.k8s.io/yaml"
	"strings"
)

func (r *CassandraClusterReconciler) reconcileCassandraConfigConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	//deployed with the operator
	operatorCM := &v1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: names.OperatorCassandraConfigCM(), Namespace: r.Cfg.Namespace}, operatorCM); err != nil {
		if kerrors.IsNotFound(err) {
			return errors.Wrap(err, "operator cassandra config configmap doesn't exist")
		}
		return errors.Wrap(err, "can't get operator cassandra config configmap")
	}

	for _, dc := range cc.Spec.DCs {
		desiredCM := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      names.ConfigMap(cc, dc.Name),
				Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
				Namespace: cc.Namespace,
			},
		}

		dcCMData := operatorCM.Data
		dcCMData["cassandra-rackdc.properties"] = fmt.Sprintf(`dc=%s
rack=RAC1
prefer_local=true`, dc.Name)
		delete(dcCMData, "nodetool-ssl.properties") //TODO will be implemented as part of TLS support

		cassandraYaml := make(map[string]interface{})
		err := yaml.Unmarshal([]byte(dcCMData["cassandra.yaml"]), &cassandraYaml)
		if err != nil {
			return errors.Wrap(err, "can't unmarshal 'cassandra.yaml'")
		}

		cassandraYaml["cluster_name"] = cc.Name
		cassandraYaml["seed_provider"] = []map[string]interface{}{
			{
				"class_name": "org.apache.cassandra.locator.SimpleSeedProvider",
				"parameters": []map[string]string{
					{
						"seeds": strings.Join(getSeedsList(cc), ","),
					},
				},
			},
		}

		cassandraYamlBytes, err := yaml.Marshal(cassandraYaml)
		if err != nil {
			return errors.Wrap(err, "can't marshal 'cassandra.yaml'")
		}

		dcCMData["cassandra.yaml"] = string(cassandraYamlBytes)
		desiredCM.Data = dcCMData
		if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
			return errors.Wrap(err, "Cannot set controller reference")
		}

		actualCM := &v1.ConfigMap{}
		err = r.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, actualCM)
		if err != nil && kerrors.IsNotFound(err) {
			r.Log.Info("Creating cassandra config ConfigMap")
			if err = r.Create(ctx, desiredCM); err != nil {
				return errors.Wrap(err, "Unable to create cassandra config ConfigMap")
			}
		} else if err != nil {
			return errors.Wrap(err, "Could not Get cassandra config ConfigMap")
		} else if !compare.EqualConfigMap(actualCM, desiredCM) {
			r.Log.Info("Updating cassandra config ConfigMap")
			r.Log.Debug(compare.DiffConfigMap(actualCM, desiredCM))
			actualCM.Labels = desiredCM.Labels
			actualCM.Data = desiredCM.Data
			if err = r.Update(ctx, actualCM); err != nil {
				return errors.Wrap(err, "Could not Update cassandra config ConfigMap")
			}
		} else {
			r.Log.Debug("No updates for cassandra config ConfigMap")

		}
	}

	return nil
}

func cassandraDCConfigVolume(cc *v1alpha1.CassandraCluster, dc v1alpha1.DC) v1.Volume {
	return v1.Volume{
		Name: "config",
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: names.ConfigMap(cc, dc.Name),
				},
				DefaultMode: proto.Int32(420),
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
