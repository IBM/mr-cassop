package controllers

import (
	"context"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	nwv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var pathType = nwv1.PathTypeImplementationSpecific

func (r *CassandraClusterReconciler) reconcileProberIngress(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredIngress := &nwv1.Ingress{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        names.ProberIngress(cc.Name),
			Namespace:   cc.Namespace,
			Labels:      labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentProber),
			Annotations: cc.Spec.Ingress.Annotations,
		},
		Spec: nwv1.IngressSpec{
			IngressClassName: cc.Spec.Ingress.IngressClassName,
			DefaultBackend:   nil,
			TLS: []nwv1.IngressTLS{
				{
					Hosts:      []string{names.ProberIngressDomain(cc.Name, cc.Spec.Ingress.Domain, cc.Namespace)},
					SecretName: cc.Spec.Ingress.Secret,
				},
			},
			Rules: []nwv1.IngressRule{
				{
					Host: names.ProberIngressDomain(cc.Name, cc.Spec.Ingress.Domain, cc.Namespace),
					IngressRuleValue: nwv1.IngressRuleValue{
						HTTP: &nwv1.HTTPIngressRuleValue{
							Paths: []nwv1.HTTPIngressPath{
								{
									PathType: &pathType,
									Path:     "/",
									Backend: nwv1.IngressBackend{
										Service: &nwv1.IngressServiceBackend{
											Name: names.ProberService(cc.Name),
											Port: nwv1.ServiceBackendPort{Number: v1alpha1.ProberServicePort},
										},
										Resource: nil,
									},
								},
							},
						},
					},
				},
			},
		},
		Status: nwv1.IngressStatus{},
	}

	if err := controllerutil.SetControllerReference(cc, desiredIngress, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualIngress := &nwv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Name: names.ProberIngress(cc.Name), Namespace: cc.Namespace}, actualIngress)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Info("Creating prober ingress")
		err = r.Create(ctx, desiredIngress)
		if err != nil {
			return errors.Wrap(err, "Failed to create ingress")
		}
	} else if err != nil {
		return errors.Wrap(err, "Failed to get ingress")
	} else {
		if !compare.EqualIngress(desiredIngress, actualIngress) {
			r.Log.Info("Updating prober ingress")
			r.Log.Debug(compare.DiffIngress(actualIngress, desiredIngress))
			actualIngress.Spec = desiredIngress.Spec
			actualIngress.Labels = desiredIngress.Labels
			actualIngress.Annotations = desiredIngress.Annotations
			if err = r.Update(ctx, actualIngress); err != nil {
				return errors.Wrap(err, "failed to updagte ingress")
			}
		} else {
			r.Log.Debugf("No updates to prober ingress")
		}
	}

	return nil
}
