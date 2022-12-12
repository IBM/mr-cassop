package controllers

import (
	"context"
	"fmt"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
)

func (r *CassandraClusterReconciler) reconcileCassandraPodLabels(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	pods, err := r.getCassandraPods(ctx, cc)
	if err != nil {
		return err
	}

	if len(pods.Items) == 0 {
		return nil
	}

	seedPodNames := make([]string, 0)
	for _, dc := range cc.Spec.DCs {
		numSeeds := dcNumberOfSeeds(cc, dc)
		for i := int32(0); i < numSeeds; i++ {
			seed := fmt.Sprintf("%s-%d", names.DC(cc.Name, dc.Name), i)
			seedPodNames = append(seedPodNames, seed)
		}
	}

	for _, pod := range pods.Items {
		_, seedLabelExists := pod.Labels[v1alpha1.CassandraClusterSeed]

		updated := false
		if seedLabelExists {
			if !util.Contains(seedPodNames, pod.Name) { // no more a seed pod, remove label
				updated = true
				delete(pod.Labels, v1alpha1.CassandraClusterSeed)
			}
		} else {
			if util.Contains(seedPodNames, pod.Name) { // seed pod without a label, add it
				updated = true
				pod.Labels[v1alpha1.CassandraClusterSeed] = pod.Name
			}
		}
		if updated {
			if err := r.Update(ctx, &pod); err != nil {
				return errors.Wrapf(err, "can't update labels for pod %s/%s", pod.Namespace, pod.Name)
			}
		}
	}

	return nil
}
