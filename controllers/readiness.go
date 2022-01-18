package controllers

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *CassandraClusterReconciler) clusterReady(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) (bool, error) {
	unreadyDCs, err := r.unreadyDCs(ctx, cc)
	if err != nil {
		return false, errors.Wrap(err, "Failed to check DCs readiness")
	}

	if len(unreadyDCs) > 0 {
		if err = proberClient.UpdateRegionStatus(ctx, false); err != nil {
			return false, errors.Wrap(err, "Failed to set DC status to not ready for prober")
		}
		r.Log.Warnf("Not all DCs are ready: %q.", unreadyDCs)
		return false, nil
	}
	r.Log.Debug("All DCs are ready")
	err = proberClient.UpdateRegionStatus(ctx, true)
	if err != nil {
		return false, errors.Wrap(err, "Failed to set DC's status to ready")
	}

	var unreadyRegions []string
	if unreadyRegions, err = r.unreadyRegions(ctx, cc, proberClient); err != nil {
		return false, errors.Wrap(err, "Failed to check region readiness")
	}

	if cc.Spec.HostPort.Enabled && len(unreadyRegions) != 0 {
		r.Log.Warnf("Not all regions are ready: %q.", unreadyRegions)
		return false, nil
	}

	return true, err
}

func (r *CassandraClusterReconciler) unreadyDCs(ctx context.Context, cc *v1alpha1.CassandraCluster) ([]string, error) {
	unreadyDCs := make([]string, 0)
	for _, dc := range cc.Spec.DCs {
		sts := &appsv1.StatefulSet{}
		err := r.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, sts)
		if err != nil {
			if apierrors.IsNotFound(err) { // happens when add a new DC and the statefulset is not created yet
				unreadyDCs = append(unreadyDCs, dc.Name)
				continue
			}
			return nil, errors.Wrap(err, "failed to get statefulset: "+sts.Name)
		}

		if *dc.Replicas != sts.Status.ReadyReplicas || (sts.Status.ReadyReplicas == 0 && *dc.Replicas != 0) {
			unreadyDCs = append(unreadyDCs, dc.Name)
		}
	}
	return unreadyDCs, nil
}

func (r *CassandraClusterReconciler) unreadyRegions(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) ([]string, error) {
	unreadyRegions := make([]string, 0)
	for _, externalRegion := range cc.Spec.ExternalRegions {
		if len(externalRegion.Domain) == 0 { //region not managed by an operator
			continue
		}
		proberHost := names.ProberIngressDomain(cc, externalRegion)
		regionReady, err := proberClient.RegionReady(ctx, proberHost)
		if err != nil {
			r.Log.Warnf(fmt.Sprintf("Unable to get DC's status from prober %q. Err: %#v", proberHost, err))
			return nil, ErrRegionNotReady
		}

		if !regionReady {
			unreadyRegions = append(unreadyRegions, proberHost)
		}
	}

	return unreadyRegions, nil
}

func (r *CassandraClusterReconciler) waitForFirstRegionReaper(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) (bool, error) {
	if len(managedExternalRegionsDomains(cc.Spec.ExternalRegions)) == 0 {
		return false, nil
	}

	allRegionsHosts := getAllRegionsHosts(cc)
	firstRegionsToInit := allRegionsHosts[0]

	// start reaper init if it's the first region to init
	if firstRegionsToInit == names.ProberIngressHost(cc.Name, cc.Namespace, cc.Spec.Ingress.Domain) {
		return false, nil
	}

	reaperReady, err := proberClient.ReaperReady(ctx, firstRegionsToInit)
	if err != nil {
		return false, errors.Wrapf(err, "Can't get reaper status from region %s", firstRegionsToInit)
	}
	if !reaperReady {
		r.Log.Infof("Reaper is not ready in region %s", firstRegionsToInit)
		return true, nil
	}

	return false, nil
}
