package controllers

import (
	"context"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileDCService(ctx context.Context, cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) error {
	svcLabels := labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)
	svcLabels = labels.WithDCLabel(svcLabels, dc.Name)
	desiredService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.DCService(cc.Name, dc.Name),
			Labels:    svcLabels,
			Namespace: cc.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "intra",
					Protocol:   v1.ProtocolTCP,
					Port:       dbv1alpha1.IntraPort,
					TargetPort: intstr.FromInt(dbv1alpha1.IntraPort),
					NodePort:   0,
				},
				{
					Name:       "tls",
					Protocol:   v1.ProtocolTCP,
					Port:       dbv1alpha1.TlsPort,
					TargetPort: intstr.FromInt(dbv1alpha1.TlsPort),
					NodePort:   0,
				},
				{
					Name:       "jmx",
					Protocol:   v1.ProtocolTCP,
					Port:       dbv1alpha1.JmxPort,
					TargetPort: intstr.FromInt(dbv1alpha1.JmxPort),
					NodePort:   0,
				},
				{
					Name:       "cql",
					Protocol:   v1.ProtocolTCP,
					Port:       dbv1alpha1.CqlPort,
					TargetPort: intstr.FromInt(dbv1alpha1.CqlPort),
					NodePort:   0,
				},
				{
					Name:       "thrift",
					Protocol:   v1.ProtocolTCP,
					Port:       dbv1alpha1.ThriftPort,
					TargetPort: intstr.FromInt(dbv1alpha1.ThriftPort),
					NodePort:   0,
				},
			},
			ClusterIP:                v1.ClusterIPNone,
			Type:                     v1.ServiceTypeClusterIP,
			SessionAffinity:          v1.ServiceAffinityNone,
			PublishNotReadyAddresses: true,
			Selector:                 svcLabels,
		},
	}

	if cc.Spec.Cassandra.Monitoring.Enabled {
		port := getJavaAgentPort(cc.Spec.Cassandra.Monitoring.Agent)
		desiredService.Spec.Ports = append(desiredService.Spec.Ports, v1.ServicePort{
			Name:       "agent",
			Protocol:   v1.ProtocolTCP,
			Port:       port,
			TargetPort: intstr.FromInt(int(port)),
			NodePort:   0,
		})
	}

	if err := controllerutil.SetControllerReference(cc, desiredService, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualService := &v1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: names.DCService(cc.Name, dc.Name), Namespace: cc.Namespace}, actualService)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Infof("Creating service for DC %q", dc.Name)
		err = r.Create(ctx, desiredService)
		if err != nil {
			return errors.Wrapf(err, "Failed to create service for DC %q", dc.Name)
		}
	} else if err != nil {
		return errors.Wrapf(err, "Failed to get service for DC %q", dc.Name)
	} else {
		// ClusterIP is immutable once created, so always enforce the same as existing
		desiredService.Spec.ClusterIP = actualService.Spec.ClusterIP
		desiredService.Spec.ClusterIPs = actualService.Spec.ClusterIPs
		desiredService.Spec.IPFamilies = actualService.Spec.IPFamilies
		desiredService.Spec.IPFamilyPolicy = actualService.Spec.IPFamilyPolicy
		desiredService.Spec.InternalTrafficPolicy = actualService.Spec.InternalTrafficPolicy
		if !compare.EqualService(desiredService, actualService) {
			r.Log.Infof("Updating service for DC %q", dc.Name)
			r.Log.Debugf(compare.DiffService(actualService, desiredService))
			actualService.Spec = desiredService.Spec
			actualService.Labels = desiredService.Labels
			actualService.Annotations = desiredService.Annotations
			if err = r.Update(ctx, actualService); err != nil {
				return errors.Wrapf(err, "failed to update service for DC %q", dc.Name)
			}
		} else {
			r.Log.Debugf("No updates to service for DC %q", dc.Name)
		}
	}

	return nil
}

func getJavaAgentPort(agent string) int32 {
	port := int32(0)
	switch agent {
	case dbv1alpha1.CassandraAgentInstaclustr:
		port = dbv1alpha1.InstaclustrPort
	case dbv1alpha1.CassandraAgentDatastax:
		port = dbv1alpha1.DatastaxPort
	case dbv1alpha1.CassandraAgentTlp:
		port = dbv1alpha1.TlpPort
	}
	return port
}
