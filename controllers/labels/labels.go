package labels

import (
	"github.com/ibm/cassandra-operator/api/v1alpha1"
)

// CombinedComponentLabels create labels for a component and inherits CassandraCluster object labels
func CombinedComponentLabels(instance *v1alpha1.CassandraCluster, componentName string) (m map[string]string) {
	inherited := InheritLabels(instance)
	generated := ComponentLabels(instance, componentName)

	m = make(map[string]string, len(inherited)+len(generated))
	for k, v := range inherited {
		m[k] = v
	}
	for k, v := range generated {
		m[k] = v
	}
	return
}

func ComponentLabels(instance *v1alpha1.CassandraCluster, componentName string) map[string]string {
	return map[string]string{
		v1alpha1.CassandraClusterInstance:  instance.Name,
		v1alpha1.CassandraClusterComponent: componentName,
	}
}

func InheritLabels(instance *v1alpha1.CassandraCluster) (m map[string]string) {
	m = make(map[string]string, len(instance.Labels))
	for k, v := range instance.Labels {
		m[k] = v
	}
	return
}

func WithDCLabel(labels map[string]string, dcName string) map[string]string {
	newLabels := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		newLabels[key] = value
	}
	newLabels[v1alpha1.CassandraClusterDC] = dcName
	return newLabels
}

func Cassandra(instance *v1alpha1.CassandraCluster) map[string]string {
	return ComponentLabels(instance, v1alpha1.CassandraClusterComponentCassandra)
}

func Prober(instance *v1alpha1.CassandraCluster) map[string]string {
	return ComponentLabels(instance, v1alpha1.CassandraClusterComponentProber)
}

func Reaper(instance *v1alpha1.CassandraCluster) map[string]string {
	return ComponentLabels(instance, v1alpha1.CassandraClusterComponentReaper)
}
