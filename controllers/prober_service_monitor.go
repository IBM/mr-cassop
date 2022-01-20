package controllers

import (
	"context"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileProberServiceMonitor(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if err := r.cleanupOldProberServiceMonitors(ctx, cc); err != nil {
		return errors.WithStack(err)
	}

	if !cc.Spec.Prober.ServiceMonitor.Enabled {
		return nil
	}

	if cc.Spec.Prober.ServiceMonitor.Namespace != "" {
		installationNamespace := cc.Spec.Prober.ServiceMonitor.Namespace
		if err := r.Get(ctx, types.NamespacedName{Name: installationNamespace}, &v1.Namespace{}); err != nil {
			if kerrors.IsNotFound(err) {
				r.Log.Warnf("Can't install ServiceMonitor. Namespace %q doesn't exist", installationNamespace)
				return nil
			}
			return errors.WithStack(err)
		}
	}

	serviceMonitor, err := r.createProberServiceMonitor(cc)
	if err != nil {
		return err
	}

	if err = r.applyServiceMonitor(ctx, serviceMonitor); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (r *CassandraClusterReconciler) cleanupOldProberServiceMonitors(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if !cc.Spec.Prober.ServiceMonitor.Enabled {
		if err := r.removeServiceMonitors(ctx, cc, dbv1alpha1.CassandraClusterComponentProber); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	serviceMonitorNamespace := cc.Namespace
	if cc.Spec.Prober.ServiceMonitor.Namespace != "" {
		serviceMonitorNamespace = cc.Spec.Prober.ServiceMonitor.Namespace
	}

	if err := r.removeOldServiceMonitors(ctx, cc, dbv1alpha1.CassandraClusterComponentProber, serviceMonitorNamespace); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (r *CassandraClusterReconciler) createProberServiceMonitor(cc *dbv1alpha1.CassandraCluster) (*unstructured.Unstructured, error) {
	serviceMonitor := &unstructured.Unstructured{}
	serviceMonitor.SetGroupVersionKind(serviceMonitorGVK)
	serviceMonitor.SetName(names.ProberService(cc.Name))
	serviceMonitor.SetLabels(labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentProber))
	serviceMonitor.SetNamespace(cc.Namespace)
	if cc.Spec.Prober.ServiceMonitor.Namespace != "" {
		serviceMonitor.SetNamespace(cc.Spec.Prober.ServiceMonitor.Namespace)
	}

	additionalLabels := cc.Spec.Prober.ServiceMonitor.Labels
	if len(additionalLabels) > 0 {
		newLabels := serviceMonitor.GetLabels()
		for key, value := range additionalLabels {
			newLabels[key] = value
		}
		serviceMonitor.SetLabels(newLabels)
	}

	matchLabels := map[string]interface{}{}
	componentLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentProber)
	for key, value := range componentLabels {
		matchLabels[key] = value
	}
	serviceMonitor.Object["spec"] = map[string]interface{}{
		"selector": map[string]interface{}{
			"matchLabels": matchLabels,
		},
		"endpoints": []interface{}{
			map[string]interface{}{
				"port":     "prober",
				"path":     "/metrics",
				"interval": cc.Spec.Prober.ServiceMonitor.ScrapeInterval,
			},
		},
		"namespaceSelector": map[string]interface{}{
			"matchNames": []interface{}{cc.Namespace},
		},
	}

	if cc.Spec.Prober.ServiceMonitor.Namespace == "" {
		err := controllerutil.SetControllerReference(cc, serviceMonitor, r.Scheme)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return serviceMonitor, nil
}
