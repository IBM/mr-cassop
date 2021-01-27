package controllers

import (
	"context"
	"fmt"
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
	"strings"
)

func (r *CassandraClusterReconciler) reconcileCassandraConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	operatorCM, err := r.getConfigMap(ctx, names.OperatorCassandraConfigCM(), r.Cfg.Namespace)
	if err != nil {
		return err
	}
	for _, dc := range cc.Spec.DCs {
		desiredCM := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      names.ConfigMap(cc.Name, dc.Name),
				Namespace: cc.Namespace,
				Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
			},
		}
		cmData, err := generateCassandraDcData(cc, dc.Name, operatorCM.Data)
		if err != nil {
			return err
		}
		desiredCM.Data = cmData
		if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
			return errors.Wrap(err, "Cannot set controller reference")
		}
		if err = r.reconcileConfigMap(ctx, desiredCM); err != nil {
			return err
		}
	}
	return nil
}

func generateCassandraDcData(cc *v1alpha1.CassandraCluster, dcName string, cmData map[string]string) (map[string]string, error) {
	data := util.MergeMap(make(map[string]string), cmData)
	cassandraYaml := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(data["cassandra.yaml"]), &cassandraYaml)
	if err != nil {
		return nil, errors.Wrap(err, "can't unmarshal 'cassandra.yaml'")
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

	if cc.Spec.Cassandra.Persistence.Enabled && cc.Spec.Cassandra.Persistence.CommitLogVolume {
		cassandraYaml["commitlog_directory"] = cassandraCommitLogDir
	}

	cassandraYamlBytes, err := yaml.Marshal(cassandraYaml)
	if err != nil {
		return nil, errors.Wrap(err, "can't marshal 'cassandra.yaml'")
	}
	data["cassandra.yaml"] = string(cassandraYamlBytes)
	data["cassandra-rackdc.properties"] = fmt.Sprintf(`dc=%s
rack=RAC1
prefer_local=true`, dcName)
	delete(data, "nodetool-ssl.properties") // TODO: will be implemented as part of TLS support
	return data, nil
}

func cassandraDCConfigVolume(cc *v1alpha1.CassandraCluster, dc v1alpha1.DC) v1.Volume {
	return v1.Volume{
		Name: "config",
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: names.ConfigMap(cc.Name, dc.Name),
				},
				DefaultMode: proto.Int32(0644),
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
