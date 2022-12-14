package controllers

import (
	"context"
	"fmt"
	"sort"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	nwv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileNetworkPolicies(ctx context.Context, cc *dbv1alpha1.CassandraCluster, proberClient prober.ProberClient, podList *v1.PodList) error {
	var err error

	if cc.Spec.NetworkPolicies.Enabled {
		if err = r.reconcileCassandraNetworkPolicies(ctx, cc, proberClient, podList); err != nil {
			return errors.Wrapf(err, "Failed to reconcile Cassandra Kubernetes Network Policies")
		}

		if err = r.reconcileProberNetworkPolicies(ctx, cc); err != nil {
			return errors.Wrapf(err, "Failed to reconcile Prober Kubernetes Network Policies")
		}

		if err = r.reconcileReaperNetworkPolicies(ctx, cc); err != nil {
			return errors.Wrapf(err, "Failed to reconcile Repaer Kubernetes Network Policies")
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileCassandraNetworkPolicies(ctx context.Context, cc *dbv1alpha1.CassandraCluster, proberClient prober.ProberClient, podList *v1.PodList) error {
	var err error

	baseCasPolicy := &nwv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterNetworkPolicy),
		},
		Spec: nwv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra},
			},
			PolicyTypes: []nwv1.PolicyType{"Ingress"},
		},
	}

	if err = r.cassandraClusterPolicy(ctx, cc, baseCasPolicy); err != nil {
		return errors.Wrap(err, "Failed to reconcile network policy")
	}

	if err = r.cassandraClusterHostportPolicy(ctx, cc, baseCasPolicy, proberClient, podList); err != nil {
		return errors.Wrap(err, "Failed to reconcile network policy")
	}

	if err = r.cassandraClusterExternalManagedRegionsPolicy(ctx, cc, baseCasPolicy, proberClient); err != nil {
		return errors.Wrap(err, "Failed to reconcile network policy")
	}

	if err = r.cassandraClusterHostPortReaperPolicy(ctx, cc, baseCasPolicy, proberClient); err != nil {
		return errors.Wrap(err, "Failed to reconcile network policy")
	}

	if err = r.cassandraClusterExtraRulesPolicy(ctx, cc, baseCasPolicy); err != nil {
		return errors.Wrap(err, "Failed to reconcile network policy")
	}

	if err = r.cassandraClusterExtraPrometheusRulesPolicy(ctx, cc, baseCasPolicy); err != nil {
		return errors.Wrap(err, "Failed to reconcile network policy")
	}

	if err = r.cassandraClusterExtraIPsPolicy(ctx, cc, baseCasPolicy); err != nil {
		return errors.Wrap(err, "Failed to reconcile network policy")
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileProberNetworkPolicies(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	var err error

	if cc.Spec.NetworkPolicies.ExtraIngressRules == nil {
		warnMsg := "Prober is not secured by network policies. You may setup `networkPolicies.extraIngressRules` to enable network policies for prober component"
		r.Events.Warning(cc, events.EventInsecureSetup, warnMsg)
		r.Log.Warn(warnMsg)
		return nil
	}

	desiredProberPolicy := &nwv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ProberNetworkPolicyName(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterNetworkPolicy),
		},
		Spec: nwv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentProber},
			},
			Ingress: []nwv1.NetworkPolicyIngressRule{
				// Allow C* pods
				{
					Ports: []nwv1.NetworkPolicyPort{
						nwPolicyPort(dbv1alpha1.ProberContainerPort),
					},
					From: []nwv1.NetworkPolicyPeer{
						nwPolicyPeer(map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra}, cc.Namespace),
					},
				},
				// Allow C* operator
				{
					Ports: []nwv1.NetworkPolicyPort{
						nwPolicyPort(dbv1alpha1.ProberContainerPort),
					},
					From: []nwv1.NetworkPolicyPeer{
						nwPolicyPeer(dbv1alpha1.CassandraOperatorPodLabels, r.Cfg.Namespace),
					},
				},
			},
			PolicyTypes: []nwv1.PolicyType{"Ingress"},
		},
	}

	if cc.Spec.NetworkPolicies.ExtraPrometheusRules != nil && cc.Spec.Prober.ServiceMonitor.Enabled {
		for _, rule := range cc.Spec.NetworkPolicies.ExtraPrometheusRules {
			desiredProberPolicy.Spec.Ingress = append(desiredProberPolicy.Spec.Ingress, nwv1.NetworkPolicyIngressRule{
				// Allow prometheus scrapes
				Ports: []nwv1.NetworkPolicyPort{
					nwPolicyPort(dbv1alpha1.ProberContainerPort),
				},
				From: []nwv1.NetworkPolicyPeer{
					{
						PodSelector:       rule.PodSelector,
						NamespaceSelector: rule.NamespaceSelector,
					},
				},
			})
		}
	}

	if cc.Spec.HostPort.Enabled && cc.Spec.NetworkPolicies.ExtraIngressRules != nil {
		for _, rule := range cc.Spec.NetworkPolicies.ExtraIngressRules {
			desiredProberPolicy.Spec.Ingress = append(desiredProberPolicy.Spec.Ingress, nwv1.NetworkPolicyIngressRule{
				// Allow ingress
				Ports: []nwv1.NetworkPolicyPort{
					nwPolicyPort(dbv1alpha1.ProberContainerPort),
				},
				From: []nwv1.NetworkPolicyPeer{
					{
						PodSelector:       rule.PodSelector,
						NamespaceSelector: rule.NamespaceSelector,
					},
				},
			})
		}
	}

	if err = r.reconcileNetworkPolicy(ctx, cc, desiredProberPolicy); err != nil {
		return errors.Wrapf(err, "Failed to create/update network policy `%s`", desiredProberPolicy.Name)
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileReaperNetworkPolicies(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	var err error

	desiredReaperPolicy := &nwv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ReaperNetworkPolicyName(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterNetworkPolicy),
		},
		Spec: nwv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentReaper},
			},
			PolicyTypes: []nwv1.PolicyType{"Ingress"},
			Ingress: []nwv1.NetworkPolicyIngressRule{
				{
					Ports: []nwv1.NetworkPolicyPort{
						nwPolicyPort(dbv1alpha1.ReaperAppPort),
					},
					From: []nwv1.NetworkPolicyPeer{
						nwPolicyPeer(dbv1alpha1.CassandraOperatorPodLabels, r.Cfg.Namespace),
					},
				},
			},
		},
	}

	if cc.Spec.NetworkPolicies.ExtraPrometheusRules != nil && cc.Spec.Reaper.ServiceMonitor.Enabled {
		for _, rule := range cc.Spec.NetworkPolicies.ExtraPrometheusRules {
			desiredReaperPolicy.Spec.Ingress = append(desiredReaperPolicy.Spec.Ingress, nwv1.NetworkPolicyIngressRule{
				// Allow prometheus scrapes
				Ports: []nwv1.NetworkPolicyPort{
					nwPolicyPort(dbv1alpha1.ReaperAppPort),
				},
				From: []nwv1.NetworkPolicyPeer{
					{
						PodSelector:       rule.PodSelector,
						NamespaceSelector: rule.NamespaceSelector,
					},
				},
			})
		}
	}

	if err = r.reconcileNetworkPolicy(ctx, cc, desiredReaperPolicy); err != nil {
		return errors.Wrapf(err, "Failed to create/update network policy `%s`", desiredReaperPolicy.Name)
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileNetworkPolicy(ctx context.Context, cc *dbv1alpha1.CassandraCluster, desiredPolicy *nwv1.NetworkPolicy) error {
	var err error
	if err = controllerutil.SetControllerReference(cc, desiredPolicy, r.Scheme); err != nil {
		return err
	}

	actualPolicy := &nwv1.NetworkPolicy{}
	err = r.Get(ctx, types.NamespacedName{Name: desiredPolicy.Name, Namespace: cc.Namespace}, actualPolicy)
	if err != nil && apierrors.IsNotFound(err) {
		if err = r.Create(ctx, desiredPolicy); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		if !compare.EqualNetworkPolicy(actualPolicy, desiredPolicy) {
			r.Log.Infof("Updating network policy %s", desiredPolicy.Name)
			r.Log.Debug(compare.DiffNetworkPolicy(actualPolicy, desiredPolicy))
			actualPolicy.Spec = desiredPolicy.Spec
			actualPolicy.Labels = desiredPolicy.Labels
			actualPolicy.Annotations = desiredPolicy.Annotations
			if err = r.Update(ctx, actualPolicy); err != nil {
				return err
			}
		} else {
			r.Log.Debugf("No updates to network policy: `%s`", actualPolicy.Name)
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) cleanupNetworkPolicies(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if !cc.Spec.NetworkPolicies.Enabled {
		nwPolicies := &nwv1.NetworkPolicyList{}
		err := r.List(ctx, nwPolicies, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterNetworkPolicy)))
		if err != nil {
			return errors.Wrapf(err, "Failed to list network policies")
		}

		for _, item := range nwPolicies.Items {
			if err = r.Delete(ctx, &item); err != nil {
				return errors.Wrapf(err, "Failed to delete network policy %s", item.Name)
			}
		}
	}
	return nil
}

func (r *CassandraClusterReconciler) cassandraClusterPolicy(ctx context.Context, cc *dbv1alpha1.CassandraCluster, baseNetworkPolicy *nwv1.NetworkPolicy) error {
	desiredClusterCasPolicy := baseNetworkPolicy.DeepCopy()

	desiredClusterCasPolicy.Name = names.CassandraClusterNetworkPolicyName(cc.Name)
	desiredClusterCasPolicy.Spec.Ingress = []nwv1.NetworkPolicyIngressRule{
		// Allow C* pods
		{
			Ports: []nwv1.NetworkPolicyPort{
				nwPolicyPort(dbv1alpha1.TlsPort),
				nwPolicyPort(dbv1alpha1.IntraPort),
				nwPolicyPort(dbv1alpha1.IcarusPort),
			},
			From: []nwv1.NetworkPolicyPeer{
				nwPolicyPeer(map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra}, cc.Namespace),
			},
		},
		// Allow C* operator
		{
			Ports: []nwv1.NetworkPolicyPort{
				nwPolicyPort(dbv1alpha1.CqlPort),
				nwPolicyPort(dbv1alpha1.IcarusPort),
			},
			From: []nwv1.NetworkPolicyPeer{
				nwPolicyPeer(dbv1alpha1.CassandraOperatorPodLabels, r.Cfg.Namespace),
			},
		},
		// Allow Prober
		{
			Ports: []nwv1.NetworkPolicyPort{
				nwPolicyPort(dbv1alpha1.JmxPort),
			},
			From: []nwv1.NetworkPolicyPeer{
				nwPolicyPeer(map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentProber}, cc.Namespace),
			},
		},
		// Allow reaper
		{
			Ports: []nwv1.NetworkPolicyPort{
				nwPolicyPort(dbv1alpha1.CqlPort),
				nwPolicyPort(dbv1alpha1.JmxPort),
			},
			From: []nwv1.NetworkPolicyPeer{
				nwPolicyPeer(map[string]string{dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentReaper}, cc.Namespace),
			},
		},
	}

	if err := r.reconcileNetworkPolicy(ctx, cc, desiredClusterCasPolicy); err != nil {
		return errors.Wrapf(err, "Failed to create/update network policy `%s`", desiredClusterCasPolicy.Name)
	}

	return nil
}

func (r *CassandraClusterReconciler) cassandraClusterHostportPolicy(ctx context.Context, cc *dbv1alpha1.CassandraCluster, baseNetworkPolicy *nwv1.NetworkPolicy, proberClient prober.ProberClient, podList *v1.PodList) error {
	var curRegionNodeIPs []string

	if cc.Spec.HostPort.Enabled {
		desiredHostPortClusterPolicy := baseNetworkPolicy.DeepCopy()
		desiredHostPortClusterPolicy.Name = names.CassandraHostPortPolicyName(cc.Name)

		if podList.Items == nil {
			return errors.Errorf("Pod list is empty")
		}

		for _, pod := range podList.Items {
			curRegionNodeIPs = append(curRegionNodeIPs, pod.Status.HostIP)
		}

		if curRegionNodeIPs == nil {
			return errors.Errorf("node ip list is empty")
		}

		if err := proberClient.UpdateRegionIPs(ctx, curRegionNodeIPs); err != nil {
			return err
		}

		desiredHostPortClusterPolicy.Spec.Ingress = append(desiredHostPortClusterPolicy.Spec.Ingress, nwv1.NetworkPolicyIngressRule{
			// Allow C* node IPs from current region
			Ports: []nwv1.NetworkPolicyPort{
				nwPolicyPort(dbv1alpha1.TlsPort),
				nwPolicyPort(dbv1alpha1.IntraPort),
				nwPolicyPort(dbv1alpha1.IcarusPort),
			},
			From: generatePeers(curRegionNodeIPs),
		})

		if err := r.reconcileNetworkPolicy(ctx, cc, desiredHostPortClusterPolicy); err != nil {
			return errors.Wrapf(err, "Failed to create/update network policy `%s`", desiredHostPortClusterPolicy.Name)
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) cassandraClusterExternalManagedRegionsPolicy(ctx context.Context, cc *dbv1alpha1.CassandraCluster, baseNetworkPolicy *nwv1.NetworkPolicy, proberClient prober.ProberClient) error {
	if cc.Spec.HostPort.Enabled && len(cc.Spec.ExternalRegions.Managed) > 0 {

		desiredManagedRegionsCasPolicy := baseNetworkPolicy.DeepCopy()
		desiredManagedRegionsCasPolicy.Name = names.CassandraExternalManagedRegionsPolicyName(cc.Name)

		casIPs := r.getExternalCassandraIPs(ctx, cc, proberClient)

		desiredManagedRegionsCasPolicy.Spec.Ingress = append(desiredManagedRegionsCasPolicy.Spec.Ingress, nwv1.NetworkPolicyIngressRule{
			// Allow C* node IPs from external region
			Ports: []nwv1.NetworkPolicyPort{
				nwPolicyPort(dbv1alpha1.TlsPort),
				nwPolicyPort(dbv1alpha1.IntraPort),
				nwPolicyPort(dbv1alpha1.IcarusPort),
			},
			From: generatePeers(casIPs),
		})

		if err := r.reconcileNetworkPolicy(ctx, cc, desiredManagedRegionsCasPolicy); err != nil {
			return errors.Wrapf(err, "Failed to create/update network policy `%s`", desiredManagedRegionsCasPolicy.Name)
		}
	}

	return nil
}

// This logic is needed to allow reaper pods from managed regions and all dcs within current region to talk to C* nodes via JMX through worker node's network
func (r *CassandraClusterReconciler) cassandraClusterHostPortReaperPolicy(ctx context.Context, cc *dbv1alpha1.CassandraCluster, baseNetworkPolicy *nwv1.NetworkPolicy, proberClient prober.ProberClient) error {
	if !*cc.Spec.NetworkPolicies.AllowReaperNodeIPs || !cc.Spec.HostPort.Enabled {
		return nil
	}

	desiredExternalReaperPolicy := baseNetworkPolicy.DeepCopy()
	desiredExternalReaperPolicy.Name = names.CassandraHostPortReaperPolicyName(cc.Name)

	reaperIPs, err := r.getExternalReaperIPs(ctx, cc, proberClient)
	if err != nil {
		return err
	}

	desiredExternalReaperPolicy.Spec.Ingress = append(desiredExternalReaperPolicy.Spec.Ingress, nwv1.NetworkPolicyIngressRule{
		// Allow Reaper IPs
		Ports: []nwv1.NetworkPolicyPort{
			nwPolicyPort(dbv1alpha1.JmxPort),
		},
		From: generatePeers(reaperIPs),
	})

	if err := r.reconcileNetworkPolicy(ctx, cc, desiredExternalReaperPolicy); err != nil {
		return errors.Wrapf(err, "Failed to create/update network policy `%s`", desiredExternalReaperPolicy.Name)
	}

	return nil
}

func (r *CassandraClusterReconciler) cassandraClusterExtraRulesPolicy(ctx context.Context, cc *dbv1alpha1.CassandraCluster, baseNetworkPolicy *nwv1.NetworkPolicy) error {
	if len(cc.Spec.NetworkPolicies.ExtraCassandraRules) > 0 {

		desiredExtraCasPolicy := baseNetworkPolicy.DeepCopy()
		desiredExtraCasPolicy.Name = names.CassandraExtraRulesPolicyName(cc.Name)

		for _, rule := range cc.Spec.NetworkPolicies.ExtraCassandraRules {

			var nwPolicyPorts []nwv1.NetworkPolicyPort

			// Expose default cql port
			if len(rule.Ports) == 0 {
				nwPolicyPorts = append(nwPolicyPorts, nwPolicyPort(dbv1alpha1.CqlPort))
			} else {
				for _, port := range rule.Ports {
					nwPolicyPorts = append(nwPolicyPorts, nwPolicyPort(port))
				}
			}

			desiredExtraCasPolicy.Spec.Ingress = append(desiredExtraCasPolicy.Spec.Ingress, nwv1.NetworkPolicyIngressRule{
				Ports: nwPolicyPorts,
				From: []nwv1.NetworkPolicyPeer{
					{
						PodSelector:       rule.PodSelector,
						NamespaceSelector: rule.NamespaceSelector,
					},
				},
			})
		}

		if err := r.reconcileNetworkPolicy(ctx, cc, desiredExtraCasPolicy); err != nil {
			return errors.Wrapf(err, "Failed to create/update network policy `%s`", desiredExtraCasPolicy.Name)
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) cassandraClusterExtraPrometheusRulesPolicy(ctx context.Context, cc *dbv1alpha1.CassandraCluster, baseNetworkPolicy *nwv1.NetworkPolicy) error {
	if len(cc.Spec.NetworkPolicies.ExtraPrometheusRules) > 0 && cc.Spec.Cassandra.Monitoring.Enabled {
		desiredPrometheusCasPolicy := baseNetworkPolicy.DeepCopy()
		desiredPrometheusCasPolicy.Name = names.CassandraExtraPrometheusRulesPolicyName(cc.Name)

		for _, rule := range cc.Spec.NetworkPolicies.ExtraPrometheusRules {
			desiredPrometheusCasPolicy.Spec.Ingress = append(desiredPrometheusCasPolicy.Spec.Ingress, nwv1.NetworkPolicyIngressRule{
				Ports: []nwv1.NetworkPolicyPort{
					nwPolicyPort(getJavaAgentPort(cc.Spec.Cassandra.Monitoring.Agent)),
				},
				From: []nwv1.NetworkPolicyPeer{
					{
						PodSelector:       rule.PodSelector,
						NamespaceSelector: rule.NamespaceSelector,
					},
				},
			})
		}

		if err := r.reconcileNetworkPolicy(ctx, cc, desiredPrometheusCasPolicy); err != nil {
			return errors.Wrapf(err, "Failed to create/update network policy `%s`", desiredPrometheusCasPolicy.Name)
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) cassandraClusterExtraIPsPolicy(ctx context.Context, cc *dbv1alpha1.CassandraCluster, baseNetworkPolicy *nwv1.NetworkPolicy) error {
	if len(cc.Spec.NetworkPolicies.ExtraCassandraIPs) > 0 {

		desiredAdditionalCasIpsPolicy := baseNetworkPolicy.DeepCopy()
		desiredAdditionalCasIpsPolicy.Name = names.CassandraExtraIpsPolicyName(cc.Name)

		desiredAdditionalCasIpsPolicy.Spec.Ingress = []nwv1.NetworkPolicyIngressRule{
			{ // Allow C* node IPs from external region
				Ports: []nwv1.NetworkPolicyPort{
					nwPolicyPort(dbv1alpha1.TlsPort),
					nwPolicyPort(dbv1alpha1.IntraPort),
				},
				From: generatePeers(cc.Spec.NetworkPolicies.ExtraCassandraIPs),
			},
		}

		if err := r.reconcileNetworkPolicy(ctx, cc, desiredAdditionalCasIpsPolicy); err != nil {
			return errors.Wrapf(err, "Failed to create/update network policy `%s`", desiredAdditionalCasIpsPolicy.Name)
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) getExternalCassandraIPs(ctx context.Context, cc *dbv1alpha1.CassandraCluster, proberClient prober.ProberClient) []string {
	var casIPs []string

	for _, managedRegion := range cc.Spec.ExternalRegions.Managed {
		proberHost := names.ProberIngressDomain(cc, managedRegion)
		managedRegionCasIPs, err := proberClient.GetRegionIPs(ctx, proberHost)
		if err == nil {
			casIPs = append(casIPs, managedRegionCasIPs...)
		} else {
			r.Log.Warnf("Failed to get C* IPs from region `https://%s/region-ips`. Network Policy rule won't be created for this region. Error: %v", proberHost, err)
		}
	}

	return casIPs
}

func (r *CassandraClusterReconciler) getExternalReaperIPs(ctx context.Context, cc *dbv1alpha1.CassandraCluster, proberClient prober.ProberClient) ([]string, error) {
	var reaperIPs []string

	reaperPods := &v1.PodList{}
	err := r.List(ctx, reaperPods, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentReaper)))
	if err != nil {
		r.Log.Error(err, "Cannot get reaper pod list")
	}

	// Obtain Reaper node IPs within the region
	for _, pod := range reaperPods.Items {
		reaperIPs = append(reaperIPs, pod.Status.HostIP)
	}

	if err = proberClient.UpdateReaperIPs(ctx, reaperIPs); err != nil {
		return nil, errors.Wrapf(err, "Failed to update reaper ips")
	}

	// Obtain Reaper node ips from external regions
	for _, managedRegion := range cc.Spec.ExternalRegions.Managed {
		proberHost := names.ProberIngressDomain(cc, managedRegion)
		externalReaperIPs, err := proberClient.GetReaperIPs(ctx, proberHost)
		if err == nil {
			reaperIPs = append(reaperIPs, externalReaperIPs...)
		} else {
			r.Log.Warnf("Failed to get Reaper IPs from region `https://%s/reaper-ips`. Network Policy rule won't be created for this region. Error: %v", proberHost, err)
		}
	}

	return reaperIPs, nil
}

func nwPolicyPort(port int32) nwv1.NetworkPolicyPort {
	var protocolTCP = v1.ProtocolTCP

	return nwv1.NetworkPolicyPort{
		Protocol: &protocolTCP,
		Port:     &intstr.IntOrString{IntVal: port},
	}
}

func nwPolicyPeer(componentLabels map[string]string, namespace string) nwv1.NetworkPolicyPeer {
	return nwv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: componentLabels,
		},
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{v1.LabelMetadataName: namespace},
		},
	}
}

func generatePeers(ips []string) []nwv1.NetworkPolicyPeer {
	var peers []nwv1.NetworkPolicyPeer

	ips = util.Uniq(ips)
	sort.Strings(ips)

	for _, ip := range ips {
		peers = append(peers, nwv1.NetworkPolicyPeer{
			IPBlock: &nwv1.IPBlock{CIDR: fmt.Sprintf("%s/32", ip)},
		})
	}

	return peers
}
