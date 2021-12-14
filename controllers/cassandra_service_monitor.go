package controllers

import (
	"context"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	v1 "k8s.io/api/core/v1"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Prometheus RelabelConfig: https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/api.md#relabelconfig
type relabelConfig struct {
	SourceLabels []interface{} `json:"sourceLabels"`
	Separator    string        `json:"separator"`
	TargetLabel  string        `json:"targetLabel"`
	Regex        string        `json:"regex"`
	Modulus      uint64        `json:"modulus"`
	Replacement  string        `json:"replacement"`
	Action       string        `json:"action"`
}

var serviceMonitorGVK = schema.GroupVersionKind{
	Group:   "monitoring.coreos.com",
	Version: "v1",
	Kind:    "ServiceMonitor",
}

var serviceMonitorListGVK = schema.GroupVersionKind{
	Group:   "monitoring.coreos.com",
	Version: "v1",
	Kind:    "ServiceMonitorList",
}

func (r *CassandraClusterReconciler) reconcileCassandraServiceMonitor(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if !cc.Spec.Monitoring.Enabled {
		return nil
	}

	if err := r.cleanupOldCassandraServiceMonitors(ctx, cc); err != nil {
		return errors.WithStack(err)
	}

	if !cc.Spec.Monitoring.ServiceMonitor.Enabled {
		return nil
	}

	if cc.Spec.Monitoring.ServiceMonitor.Namespace != "" {
		installationNamespace := cc.Spec.Monitoring.ServiceMonitor.Namespace
		if err := r.Get(ctx, types.NamespacedName{Name: installationNamespace}, &v1.Namespace{}); err != nil {
			if kerrors.IsNotFound(err) {
				r.Log.Warnf("Can't install ServiceMonitor. Namespace %q doesn't exist", installationNamespace)
				return nil
			}
			return errors.WithStack(err)
		}
	}

	serviceMonitor, err := r.createCassandraServiceMonitor(cc)
	if err != nil {
		return err
	}

	if err = r.applyServiceMonitor(ctx, serviceMonitor); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (r *CassandraClusterReconciler) applyServiceMonitor(ctx context.Context, desiredSM *unstructured.Unstructured) error {
	actualSM := &unstructured.Unstructured{}
	actualSM.SetGroupVersionKind(serviceMonitorGVK)
	err := r.Get(ctx, types.NamespacedName{Namespace: desiredSM.GetNamespace(), Name: desiredSM.GetName()}, actualSM)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Infof("Creating ServiceMonitor %s/%s", desiredSM.GetNamespace(), desiredSM.GetName())
		if err = r.Create(ctx, desiredSM); err != nil {
			return errors.Wrap(err, "Unable to create ServiceMonitor")
		}
	} else if _, ok := errors.Cause(err).(*meta.NoKindMatchError); ok {
		r.Log.Warn("ServiceMonitor can't be installed. Prometheus operator needs to be installed first")
		return nil
	} else if err != nil {
		return errors.Wrap(err, "Could not get ServiceMonitor")
	} else if !compare.EqualServiceMonitor(actualSM, desiredSM) {
		r.Log.Infof("Updating ServiceMonitor %s/%s", actualSM.GetNamespace(), actualSM.GetName())
		r.Log.Debug(compare.DiffServiceMonitor(actualSM, desiredSM))
		actualSM.Object["spec"] = desiredSM.Object["spec"]
		actualSM.SetLabels(desiredSM.GetLabels())
		if err = r.Update(ctx, actualSM); err != nil {
			return errors.Wrap(err, "Unable to update ServiceMonitor")
		}
	} else {
		r.Log.Debugw("No updates for ServiceMonitor")
	}
	return nil
}

// Deletes ServiceMonitors that are no longer needed. For example if the namespace is changed, the resource from the previous namespace should be deleted.
// If the ServiceMonitor installation is disabled through the spec, we also need to delete the ServiceMonitor.
func (r *CassandraClusterReconciler) cleanupOldCassandraServiceMonitors(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if !cc.Spec.Monitoring.ServiceMonitor.Enabled {
		if err := r.removeServiceMonitors(ctx, cc, dbv1alpha1.CassandraClusterComponentCassandra); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	serviceMonitorNamespace := cc.Namespace
	if cc.Spec.Monitoring.ServiceMonitor.Namespace != "" {
		serviceMonitorNamespace = cc.Spec.Monitoring.ServiceMonitor.Namespace
	}

	if err := r.removeOldServiceMonitors(ctx, cc, dbv1alpha1.CassandraClusterComponentCassandra, serviceMonitorNamespace); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (r *CassandraClusterReconciler) removeOldServiceMonitors(ctx context.Context, cc *dbv1alpha1.CassandraCluster, label, serviceMonitorNamespace string) error {
	serviceMonitorList := &unstructured.UnstructuredList{}
	serviceMonitorList.SetGroupVersionKind(serviceMonitorListGVK)
	err := r.List(ctx, serviceMonitorList, client.MatchingLabels(labels.CombinedComponentLabels(cc, label)))
	if err != nil {
		if _, ok := errors.Cause(err).(*meta.NoKindMatchError); ok {
			return nil
		}
		return errors.WithStack(err)
	}
	for _, item := range serviceMonitorList.Items {
		if item.GetNamespace() != serviceMonitorNamespace {
			r.Log.Infof("Deleting ServiceMonitor %s/%s", item.GetNamespace(), item.GetName())
			if err = r.Delete(ctx, &item); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

func (r *CassandraClusterReconciler) removeServiceMonitors(ctx context.Context, cc *dbv1alpha1.CassandraCluster, label string) error {
	serviceMonitorList := &unstructured.UnstructuredList{}
	serviceMonitorList.SetGroupVersionKind(serviceMonitorListGVK)
	err := r.List(ctx, serviceMonitorList, client.MatchingLabels(labels.CombinedComponentLabels(cc, label)))
	if err != nil {
		if _, ok := errors.Cause(err).(*meta.NoKindMatchError); ok {
			return nil
		}
		return errors.WithStack(err)
	}

	for _, item := range serviceMonitorList.Items {
		r.Log.Infow("Deleting ServiceMonitor %s/%s", item.GetNamespace(), item.GetName())
		if err = r.Delete(ctx, &item); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) createCassandraServiceMonitor(cc *dbv1alpha1.CassandraCluster) (*unstructured.Unstructured, error) {
	serviceMonitor := &unstructured.Unstructured{}
	serviceMonitor.SetGroupVersionKind(serviceMonitorGVK)
	serviceMonitor.SetName(cc.Name)
	serviceMonitor.SetLabels(labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra))

	serviceMonitor.SetNamespace(cc.Namespace)
	if cc.Spec.Monitoring.ServiceMonitor.Namespace != "" {
		serviceMonitor.SetNamespace(cc.Spec.Monitoring.ServiceMonitor.Namespace)
	}

	additionalLabels := cc.Spec.Monitoring.ServiceMonitor.Labels
	if len(additionalLabels) > 0 {
		newLabels := serviceMonitor.GetLabels()
		for key, value := range additionalLabels {
			newLabels[key] = value
		}
		serviceMonitor.SetLabels(newLabels)
	}

	matchLabels := map[string]interface{}{}
	componentLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)
	for key, value := range componentLabels {
		matchLabels[key] = value
	}
	serviceMonitor.Object["spec"] = map[string]interface{}{
		"selector": map[string]interface{}{
			"matchLabels": matchLabels,
		},
		"endpoints": []interface{}{
			map[string]interface{}{
				"port":     "agent",
				"interval": cc.Spec.Monitoring.ServiceMonitor.ScrapeInterval,
			},
		},
		"namespaceSelector": map[string]interface{}{
			"matchNames": []interface{}{cc.Namespace},
		},
	}
	relabelings := getCassandraServiceMonitorRelabelings(cc)
	metricRelabelings := getCassandraServiceMonitorMetricRelabelings(cc)
	if len(relabelings) > 0 {
		serviceMonitor.Object["spec"].(map[string]interface{})["endpoints"].([]interface{})[0].(map[string]interface{})["relabelings"] = relabelings
	}
	if len(metricRelabelings) > 0 {
		serviceMonitor.Object["spec"].(map[string]interface{})["endpoints"].([]interface{})[0].(map[string]interface{})["metricRelabelings"] = metricRelabelings
	}

	// Set reference only if the ServiceMonitor is installed in the same namespace. It can't be set cross namespace.
	if cc.Spec.Monitoring.ServiceMonitor.Namespace == "" {
		err := controllerutil.SetControllerReference(cc, serviceMonitor, r.Scheme)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return serviceMonitor, nil
}

func createRelabeling(config relabelConfig) map[string]interface{} {
	relabeling := map[string]interface{}{}
	if len(config.SourceLabels) > 0 {
		relabeling["sourceLabels"] = config.SourceLabels
	}
	if config.Separator != "" {
		relabeling["separator"] = config.Separator
	}
	if config.TargetLabel != "" {
		relabeling["targetLabel"] = config.TargetLabel
	}
	if config.Regex != "" {
		relabeling["regex"] = config.Regex
	}
	if config.Replacement != "" {
		relabeling["replacement"] = config.Replacement
	}
	if config.Action != "" {
		relabeling["action"] = config.Action
	}
	if config.Modulus != 0 {
		relabeling["modulus"] = config.Modulus
	}
	return relabeling
}

func getCassandraServiceMonitorRelabelings(cc *dbv1alpha1.CassandraCluster) []interface{} {
	var relabelings []interface{}
	switch cc.Spec.Monitoring.Agent {
	case dbv1alpha1.CassandraAgentDatastax:
		relabelings = []interface{}{
			createRelabeling(relabelConfig{
				Action: "labelmap",
				Regex:  "__meta_kubernetes_pod_label_(.+)",
			}),
			createRelabeling(relabelConfig{
				Action:       "replace",
				TargetLabel:  "instance",
				SourceLabels: []interface{}{"__meta_kubernetes_pod_name"},
			}),
		}

	case dbv1alpha1.CassandraAgentInstaclustr:
		relabelings = []interface{}{
			createRelabeling(relabelConfig{
				Action: "labelmap",
				Regex:  "__meta_kubernetes_pod_label_(.+)",
			}),
			createRelabeling(relabelConfig{
				Action:       "replace",
				TargetLabel:  "cassandra_node",
				SourceLabels: []interface{}{"__meta_kubernetes_pod_name"},
			}),
		}
	case dbv1alpha1.CassandraAgentTlp:
		relabelings = []interface{}{
			createRelabeling(relabelConfig{
				Regex:  "__meta_kubernetes_pod_label_(.+)",
				Action: "labelmap",
			}),
			createRelabeling(relabelConfig{
				Action:       "replace",
				TargetLabel:  "host",
				SourceLabels: []interface{}{"__meta_kubernetes_pod_name"},
			}),
		}
	}
	return relabelings
}

func getCassandraServiceMonitorMetricRelabelings(cc *dbv1alpha1.CassandraCluster) []interface{} {
	var metricRelabelings []interface{}
	if cc.Spec.Monitoring.Agent == dbv1alpha1.CassandraAgentDatastax {
		metricRelabelings = []interface{}{
			createRelabeling(relabelConfig{
				// Drop metrics we can calculate from prometheus directly
				SourceLabels: []interface{}{"__name__"},
				Regex:        ".*rate_(mean|1m|5m|15m)",
				Action:       "drop",
			}),
			createRelabeling(relabelConfig{
				// Save the original name for all metrics
				SourceLabels: []interface{}{"__name__"},
				Regex:        "(collectd_mcac_.+)",
				TargetLabel:  "prom_name",
				Replacement:  "${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"prom_name"},
				Regex:        ".+_bucket_(\\d+)",
				TargetLabel:  "le",
				Replacement:  "${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"prom_name"},
				Regex:        ".+_bucket_inf",
				TargetLabel:  "le",
				Replacement:  "+Inf",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"prom_name"},
				Regex:        ".*_histogram_p(\\d+)",
				TargetLabel:  "quantile",
				Replacement:  ".${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"prom_name"},
				Regex:        ".*_histogram_min",
				TargetLabel:  "quantile",
				Replacement:  "0",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"prom_name"},
				Regex:        ".*_histogram_max",
				TargetLabel:  "quantile",
				Replacement:  "1",
			}),
			createRelabeling(relabelConfig{
				// Table Metrics *ALL* we can drop
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.table\\.(\\w+)",
				Action:       "drop",
			}),
			createRelabeling(relabelConfig{
				// Table Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.table\\.(\\w+)\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "table",
				Replacement:  "${3}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.table\\.(\\w+)\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "keyspace",
				Replacement:  "${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.table\\.(\\w+)\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_table_${1}",
			}),
			createRelabeling(relabelConfig{
				// Keyspace Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.keyspace\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "keyspace",
				Replacement:  "${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.keyspace\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_keyspace_${1}",
			}),
			createRelabeling(relabelConfig{
				// ThreadPool Metrics (one type is repair task, so we just ignore the second part)
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.thread_pools\\.(\\w+)\\.(\\w+)\\.(\\w+).*",
				TargetLabel:  "pool_type",
				Replacement:  "${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.thread_pools\\.(\\w+)\\.(\\w+)\\.(\\w+).*",
				TargetLabel:  "pool_name",
				Replacement:  "${3}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.thread_pools\\.(\\w+)\\.(\\w+)\\.(\\w+).*",
				TargetLabel:  "__name__",
				Replacement:  "mcac_thread_pools_${1}",
			}),
			createRelabeling(relabelConfig{
				// ClientRequest Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.client_request\\.(\\w+)\\.(\\w+)$",
				TargetLabel:  "request_type",
				Replacement:  "${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.client_request\\.(\\w+)\\.(\\w+)$",
				TargetLabel:  "__name__",
				Replacement:  "mcac_client_request_${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.client_request\\.(\\w+)\\.(\\w+)\\.(\\w+)$",
				TargetLabel:  "cl",
				Replacement:  "${3}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.client_request\\.(\\w+)\\.(\\w+)\\.(\\w+)$",
				TargetLabel:  "request_type",
				Replacement:  "${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.client_request\\.(\\w+)\\.(\\w+)\\.(\\w+)$",
				TargetLabel:  "__name__",
				Replacement:  "mcac_client_request_${1}_cl",
			}),
			createRelabeling(relabelConfig{
				// Cache Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.cache\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "cache_name",
				Replacement:  "${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.cache\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_cache_${1}",
			}),
			createRelabeling(relabelConfig{
				// CQL Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.cql\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_cql_${1}",
			}),
			createRelabeling(relabelConfig{
				// Dropped Message Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.dropped_message\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "message_type",
				Replacement:  "${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.dropped_message\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_dropped_message_${1}",
			}),
			createRelabeling(relabelConfig{
				// Streaming Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.streaming\\.(\\w+)\\.(.+)$",
				TargetLabel:  "peer_ip",
				Replacement:  "${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.streaming\\.(\\w+)\\.(.+)$",
				TargetLabel:  "__name__",
				Replacement:  "mcac_streaming_${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.streaming\\.(\\w+)$",
				TargetLabel:  "__name__",
				Replacement:  "mcac_streaming_${1}",
			}),
			createRelabeling(relabelConfig{
				// CommitLog Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.commit_log\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_commit_log_${1}",
			}),
			createRelabeling(relabelConfig{
				// Compaction Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.compaction\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_compaction_${1}",
			}),
			createRelabeling(relabelConfig{
				// Storage Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.storage\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_storage_${1}",
			}),
			createRelabeling(relabelConfig{
				// Batch Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.batch\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_batch_${1}",
			}),
			createRelabeling(relabelConfig{
				// Client Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.client\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_client_${1}",
			}),
			createRelabeling(relabelConfig{
				// BufferPool Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.buffer_pool\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_buffer_pool_${1}",
			}),
			createRelabeling(relabelConfig{
				// Index Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.index\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_sstable_index_${1}",
			}),
			createRelabeling(relabelConfig{
				// HintService Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.hinted_hand_off_manager\\.([^\\-]+)-(\\w+)",
				TargetLabel:  "peer_ip",
				Replacement:  "${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.hinted_hand_off_manager\\.([^\\-]+)-(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_hints_${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.hints_service\\.hints_delays\\-(\\w+)",
				TargetLabel:  "peer_ip",
				Replacement:  "${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.hints_service\\.hints_delays\\-(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_hints_hints_delays",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.hints_service\\.([^\\-]+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_hints_${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.hints_service\\.hints_created\\.([^\\-]+)",
				TargetLabel:  "peer_ip",
				Replacement:  "${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.hints_service\\.hints_created\\.([^\\-]+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_hints_created",
			}),
			createRelabeling(relabelConfig{
				// Misc
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.memtable_pool\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_memtable_pool_${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "com\\.datastax\\.bdp\\.type\\.performance_objects\\.name\\.cql_slow_log\\.metrics\\.queries_latency",
				TargetLabel:  "__name__",
				Replacement:  "mcac_cql_slow_log_query_latency",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.read_coordination\\.(.*)",
				TargetLabel:  "read_type",
				Replacement:  "$1",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "org\\.apache\\.cassandra\\.metrics\\.read_coordination\\.(.*)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_read_coordination_requests",
			}),
			createRelabeling(relabelConfig{
				// GC Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "jvm\\.gc\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "collector_type",
				Replacement:  "${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "jvm\\.gc\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_jvm_gc_${2}",
			}),
			createRelabeling(relabelConfig{
				// JVM Metrics
				SourceLabels: []interface{}{"mcac"},
				Regex:        "jvm\\.memory\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "memory_type",
				Replacement:  "${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "jvm\\.memory\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_jvm_memory_${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "jvm\\.memory\\.pools\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "pool_name",
				Replacement:  "${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "jvm\\.memory\\.pools\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_jvm_memory_pool_${2}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "jvm\\.fd\\.usage",
				TargetLabel:  "__name__",
				Replacement:  "mcac_jvm_fd_usage",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "jvm\\.buffers\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "buffer_type",
				Replacement:  "${1}",
			}),
			createRelabeling(relabelConfig{
				SourceLabels: []interface{}{"mcac"},
				Regex:        "jvm\\.buffers\\.(\\w+)\\.(\\w+)",
				TargetLabel:  "__name__",
				Replacement:  "mcac_jvm_buffer_${2}",
			}),
			createRelabeling(relabelConfig{
				// Append the prom types back to formatted names
				SourceLabels: []interface{}{"__name__", "prom_name"},
				Regex:        "(mcac_.*);.*(_micros_bucket|_bucket|_micros_count_total|_count_total|_total|_micros_sum|_sum|_stddev).*",
				Separator:    ";",
				TargetLabel:  "__name__",
				Replacement:  "${1}${2}",
			}),
			createRelabeling(relabelConfig{
				Regex:  "prom_name",
				Action: "labeldrop",
			}),
		}
	}
	return metricRelabelings
}
