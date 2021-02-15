package controllers

import (
	"context"
	"fmt"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileCassandraPodsConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {

	podList := &v1.PodList{}
	cassandraClusterPodLabels := labels.ComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra)
	err := r.List(context.Background(), podList, client.InNamespace(cc.Namespace), client.MatchingLabels(cassandraClusterPodLabels))
	if err != nil {
		return errors.Wrap(err, "Cannot get Cassandra pods list")
	}

	nodeList := &v1.NodeList{}
	err = r.List(context.Background(), nodeList)
	if err != nil {
		return errors.Wrap(err, "Cannot get cluster nodes list")
	}

	cmData := make(map[string]string)

	// Loop through all C* pods
	for _, p := range podList.Items {
		nodeName := p.Spec.NodeName
		podName := p.Name

		if cc.Spec.Cassandra.ZonesAsRacks {
			// Find the node where the pod was started
			for _, n := range nodeList.Items {
				if n.Name == nodeName {
					nodeLabels := n.Labels
					cmData[podName+".env"] = fmt.Sprintln("export CASSANDRA_RACK=" + nodeLabels["topology.kubernetes.io/zone"])
				}
			}
		} else {
			cmData[podName+".env"] = ""
		}
	}

	desiredCM := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.PodsConfigConfigmap(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
		Data: cmData,
	}

	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	if err := r.reconcileConfigMap(ctx, desiredCM); err != nil {
		return err
	}
	return nil
}
