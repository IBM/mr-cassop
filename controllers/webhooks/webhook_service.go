package webhooks

import (
	"context"
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

func setupWebhookService(kubeClient *kubernetes.Clientset, operatorConfig *config.Config) error {
	var (
		ctx            = context.Background()
		deploymentName = "cassandra-operator"
	)

	operatorDeployment, err := kubeClient.AppsV1().Deployments(operatorConfig.Namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get deployment: %s", deploymentName)
	}

	desiredWebhookService := &v1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.WebhooksServiceName(),
			Namespace: operatorConfig.Namespace,
			Labels:    map[string]string{"operator": "cassandra-operator"},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       deploymentName,
					UID:        operatorDeployment.UID,
					Controller: proto.Bool(true),
				},
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Protocol: "TCP",
					Port:     443,
					TargetPort: intstr.IntOrString{
						IntVal: operatorConfig.WebhooksPort,
					},
				},
			},
			Selector: map[string]string{"operator": "cassandra-operator"},
			Type:     v1.ServiceTypeClusterIP,
		},
	}

	// If container crashes or restarted Service preserves in namespace
	serviceAPI := kubeClient.CoreV1().Services(operatorConfig.Namespace)
	actualWebhookService, err := serviceAPI.Get(ctx, names.WebhooksServiceName(), metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			_, err = serviceAPI.Create(ctx, desiredWebhookService, metav1.CreateOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to create Webhooks Service: %s", desiredWebhookService.Name)
			}
			return nil
		}

		return errors.Wrapf(err, "failed to get Webhooks Service: %s", names.WebhooksServiceName())
	}

	// Update existing Service
	actualWebhookService.Labels = util.MergeMap(actualWebhookService.Labels, desiredWebhookService.Labels)
	actualWebhookService.Annotations = util.MergeMap(actualWebhookService.Annotations, desiredWebhookService.Annotations)
	actualWebhookService.Spec = desiredWebhookService.Spec
	actualWebhookService.OwnerReferences = desiredWebhookService.OwnerReferences
	_, err = serviceAPI.Update(ctx, actualWebhookService, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to update Webhooks Service: %s", actualWebhookService.Name)
	}

	return nil
}

func deleteWebhookService(kubeClient *kubernetes.Clientset, operatorConfig *config.Config) error {
	var ctx = context.Background()
	serviceAPI := kubeClient.CoreV1().Services(operatorConfig.Namespace)
	err := serviceAPI.Delete(ctx, names.WebhooksServiceName(), metav1.DeleteOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return nil
}
