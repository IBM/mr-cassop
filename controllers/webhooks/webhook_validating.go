package webhooks

import (
	"context"
	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/certs"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	rbac "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func setupValidatingWebhookConfig(kubeClient *kubernetes.Clientset, operatorConfig *config.Config, caKeypair certs.Keypair) error {
	var (
		ctx             = context.Background()
		clusterRoleName = "cassandra-operator"
	)

	clusterRole, err := kubeClient.RbacV1().ClusterRoles().Get(ctx, clusterRoleName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get ClusterRole: %s", clusterRoleName)
	}

	desiredValidatingWebhookConfig := CreateValidatingWebhookConf(operatorConfig.Namespace, clusterRole, caKeypair.Crt)

	// If container crashes or restarted Validating WebhookConfig preserves in namespace
	webhookAPI := kubeClient.AdmissionregistrationV1()
	actualValidatingWebhookConfig, err := webhookAPI.ValidatingWebhookConfigurations().Get(ctx, names.ValidatingWebhookName(), metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			_, err = webhookAPI.ValidatingWebhookConfigurations().Create(ctx, &desiredValidatingWebhookConfig, metav1.CreateOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to create Validating Webhooks Configuration: %s", desiredValidatingWebhookConfig.Name)
			}
			return nil
		}

		return errors.Wrapf(err, "failed to get Validating Webhooks Configuration: %s", names.ValidatingWebhookName())
	}

	// Update existing Validating WebhookConfig
	actualValidatingWebhookConfig.Labels = util.MergeMap(actualValidatingWebhookConfig.Labels, desiredValidatingWebhookConfig.Labels)
	actualValidatingWebhookConfig.Annotations = util.MergeMap(actualValidatingWebhookConfig.Annotations, desiredValidatingWebhookConfig.Annotations)
	actualValidatingWebhookConfig.Webhooks = desiredValidatingWebhookConfig.Webhooks
	actualValidatingWebhookConfig.OwnerReferences = desiredValidatingWebhookConfig.OwnerReferences
	_, err = webhookAPI.ValidatingWebhookConfigurations().Update(ctx, actualValidatingWebhookConfig, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to update Validating Webhooks Configuration: %s", actualValidatingWebhookConfig.Name)
	}

	return nil
}

func CreateValidatingWebhookConf(namespace string, clusterRole *rbac.ClusterRole, caCrtBytes []byte) admissionv1.ValidatingWebhookConfiguration {
	var (
		sideEffectNone    = admissionv1.SideEffectClassNone
		failurePolicyType = admissionv1.Fail
		namespacedScope   = admissionv1.NamespacedScope
		ccWebhookPath     = "/validate-db-ibm-com-v1alpha1-cassandracluster"
		cbWebhookPath     = "/validate-db-ibm-com-v1alpha1-cassandrabackup"
		crWebhookPath     = "/validate-db-ibm-com-v1alpha1-cassandrarestore"
	)

	return admissionv1.ValidatingWebhookConfiguration{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:              names.ValidatingWebhookName(),
			CreationTimestamp: metav1.Time{},
			Labels:            dbv1alpha1.CassandraOperatorPodLabels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "ClusterRole",
					Name:       clusterRole.Name,
					UID:        clusterRole.UID,
				},
			},
		},
		Webhooks: []admissionv1.ValidatingWebhook{
			{
				Name: "vcassandracluster.kb.io",
				ClientConfig: admissionv1.WebhookClientConfig{
					URL: nil,
					Service: &admissionv1.ServiceReference{
						Namespace: namespace,
						Name:      names.WebhooksServiceName(),
						Path:      &ccWebhookPath,
						Port:      proto.Int32(443),
					},
					CABundle: caCrtBytes,
				},
				Rules: []admissionv1.RuleWithOperations{
					{
						Operations: []admissionv1.OperationType{admissionv1.Create, admissionv1.Update},
						Rule: admissionv1.Rule{
							APIGroups:   []string{"db.ibm.com"},
							APIVersions: []string{"v1alpha1"},
							Resources:   []string{"cassandraclusters"},
							Scope:       &namespacedScope,
						},
					},
				},
				FailurePolicy:           &failurePolicyType,
				MatchPolicy:             nil,
				NamespaceSelector:       nil,
				ObjectSelector:          nil,
				SideEffects:             &sideEffectNone,
				TimeoutSeconds:          nil,
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
			},
			{
				Name: "vcassandrabackup.kb.io",
				ClientConfig: admissionv1.WebhookClientConfig{
					URL: nil,
					Service: &admissionv1.ServiceReference{
						Namespace: namespace,
						Name:      names.WebhooksServiceName(),
						Path:      &cbWebhookPath,
						Port:      proto.Int32(443),
					},
					CABundle: caCrtBytes,
				},
				Rules: []admissionv1.RuleWithOperations{
					{
						Operations: []admissionv1.OperationType{admissionv1.Create, admissionv1.Update},
						Rule: admissionv1.Rule{
							APIGroups:   []string{"db.ibm.com"},
							APIVersions: []string{"v1alpha1"},
							Resources:   []string{"cassandrabackups"},
							Scope:       &namespacedScope,
						},
					},
				},
				FailurePolicy:           &failurePolicyType,
				MatchPolicy:             nil,
				NamespaceSelector:       nil,
				ObjectSelector:          nil,
				SideEffects:             &sideEffectNone,
				TimeoutSeconds:          nil,
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
			},
			{
				Name: "vcassandrarestore.kb.io",
				ClientConfig: admissionv1.WebhookClientConfig{
					URL: nil,
					Service: &admissionv1.ServiceReference{
						Namespace: namespace,
						Name:      names.WebhooksServiceName(),
						Path:      &crWebhookPath,
						Port:      proto.Int32(443),
					},
					CABundle: caCrtBytes,
				},
				Rules: []admissionv1.RuleWithOperations{
					{
						Operations: []admissionv1.OperationType{admissionv1.Create, admissionv1.Update},
						Rule: admissionv1.Rule{
							APIGroups:   []string{"db.ibm.com"},
							APIVersions: []string{"v1alpha1"},
							Resources:   []string{"cassandrarestores"},
							Scope:       &namespacedScope,
						},
					},
				},
				FailurePolicy:           &failurePolicyType,
				MatchPolicy:             nil,
				NamespaceSelector:       nil,
				ObjectSelector:          nil,
				SideEffects:             &sideEffectNone,
				TimeoutSeconds:          nil,
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
			},
		},
	}
}

func deleteValidatingWebhookConfig(kubeClient *kubernetes.Clientset) error {
	var ctx = context.Background()
	webhookAPI := kubeClient.AdmissionregistrationV1()
	err := webhookAPI.ValidatingWebhookConfigurations().Delete(ctx, names.ValidatingWebhookName(), metav1.DeleteOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return nil
}
