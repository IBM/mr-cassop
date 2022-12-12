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

func (r *CassandraClusterReconciler) reconcileReaperServiceMonitor(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if err := r.cleanupOldReaperServiceMonitors(ctx, cc); err != nil {
		return errors.WithStack(err)
	}

	if !cc.Spec.Reaper.ServiceMonitor.Enabled {
		return nil
	}

	if cc.Spec.Reaper.ServiceMonitor.Namespace != "" {
		installationNamespace := cc.Spec.Reaper.ServiceMonitor.Namespace
		if err := r.Get(ctx, types.NamespacedName{Name: installationNamespace}, &v1.Namespace{}); err != nil {
			if kerrors.IsNotFound(err) {
				r.Log.Warnf("Can't install ServiceMonitor. Namespace %q doesn't exist", installationNamespace)
				return nil
			}
			return errors.WithStack(err)
		}
	}

	serviceMonitor, err := r.createReaperServiceMonitor(cc)
	if err != nil {
		return err
	}

	if err = r.applyServiceMonitor(ctx, serviceMonitor); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (r *CassandraClusterReconciler) cleanupOldReaperServiceMonitors(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if !cc.Spec.Reaper.ServiceMonitor.Enabled {
		if err := r.removeServiceMonitors(ctx, cc, dbv1alpha1.CassandraClusterComponentReaper); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	serviceMonitorNamespace := cc.Namespace
	if cc.Spec.Reaper.ServiceMonitor.Namespace != "" {
		serviceMonitorNamespace = cc.Spec.Reaper.ServiceMonitor.Namespace
	}

	if err := r.removeOldServiceMonitors(ctx, cc, dbv1alpha1.CassandraClusterComponentReaper, serviceMonitorNamespace); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (r *CassandraClusterReconciler) createReaperServiceMonitor(cc *dbv1alpha1.CassandraCluster) (*unstructured.Unstructured, error) {
	serviceMonitor := &unstructured.Unstructured{}
	serviceMonitor.SetGroupVersionKind(serviceMonitorGVK)
	serviceMonitor.SetName(names.ReaperService(cc.Name))
	serviceMonitor.SetLabels(labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentReaper))
	serviceMonitor.SetNamespace(cc.Namespace)
	if cc.Spec.Reaper.ServiceMonitor.Namespace != "" {
		serviceMonitor.SetNamespace(cc.Spec.Reaper.ServiceMonitor.Namespace)
	}

	additionalLabels := cc.Spec.Reaper.ServiceMonitor.Labels
	if len(additionalLabels) > 0 {
		newLabels := serviceMonitor.GetLabels()
		for key, value := range additionalLabels {
			newLabels[key] = value
		}
		serviceMonitor.SetLabels(newLabels)
	}

	matchLabels := map[string]interface{}{}
	componentLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentReaper)
	for key, value := range componentLabels {
		matchLabels[key] = value
	}
	serviceMonitor.Object["spec"] = map[string]interface{}{
		"selector": map[string]interface{}{
			"matchLabels": matchLabels,
		},
		"endpoints": []interface{}{
			map[string]interface{}{
				"port":              "admin",
				"path":              "/prometheusMetrics",
				"interval":          cc.Spec.Reaper.ServiceMonitor.ScrapeInterval,
				"metricRelabelings": getReaperServiceMonitorMetricRelabelings(),
			},
		},
		"namespaceSelector": map[string]interface{}{
			"matchNames": []interface{}{cc.Namespace},
		},
	}

	if cc.Spec.Reaper.ServiceMonitor.Namespace == "" {
		err := controllerutil.SetControllerReference(cc, serviceMonitor, r.Scheme)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return serviceMonitor, nil
}

func getReaperServiceMonitorMetricRelabelings() []interface{} {
	return []interface{}{
		createRelabeling(relabelConfig{ // Drop metric which doesn't contain keyspace
			SourceLabels: []interface{}{"__name__"},
			Regex:        "(.*)_RepairRunner_millisSinceLastRepair_([^_]+)_([^_]+)$",
			Action:       "drop",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        "(.*)_RepairRunner_millisSinceLastRepair_(.*)_(.*)_(.*)",
			TargetLabel:  "cluster",
			Replacement:  "${2}",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        "(.*)_RepairRunner_millisSinceLastRepair_(.*)_(.*)_(.*)",
			TargetLabel:  "keyspace",
			Replacement:  "${3}",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        "(.*)_RepairRunner_millisSinceLastRepair_(.*)_(.*)_(.*)",
			TargetLabel:  "runid",
			Replacement:  "${4}",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        "(.*)_RepairRunner_millisSinceLastRepair_.*",
			TargetLabel:  "__name__",
			Replacement:  "RepairRunner_millisSinceLastRepair",
		}),

		createRelabeling(relabelConfig{ // Drop metric which doesn't contain keyspace
			SourceLabels: []interface{}{"__name__"},
			Regex:        "(.*)_RepairRunner_repairProgress_([^_]+)_([^_]+)$",
			Action:       "drop",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        "(.*)_RepairRunner_repairProgress_(.*)_(.*)_(.*)",
			TargetLabel:  "cluster",
			Replacement:  "${2}",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        "(.*)_RepairRunner_repairProgress_(.*)_(.*)_(.*)",
			TargetLabel:  "keyspace",
			Replacement:  "${3}",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        "(.*)_RepairRunner_repairProgress_(.*)_(.*)_(.*)",
			TargetLabel:  "runid",
			Replacement:  "${4}",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        ".*_RepairRunner_repairProgress_.*",
			TargetLabel:  "__name__",
			Replacement:  "RepairRunner_repairProgress",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        "com_datastax_driver_core_Cluster_([^_]+)_(.*)",
			TargetLabel:  "cluster",
			Replacement:  "${1}",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        "com_datastax_driver_core_Cluster_([^_]+)_(.*)",
			TargetLabel:  "metric",
			Replacement:  "${2}",
		}),
		createRelabeling(relabelConfig{
			SourceLabels: []interface{}{"__name__"},
			Regex:        "com_datastax_driver_core_Cluster_.*",
			TargetLabel:  "__name__",
			Replacement:  "core_Cluster",
		}),
	}
}
