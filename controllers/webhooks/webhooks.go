package webhooks

import (
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
)

func CreateWebhookAssets(kubeClient *kubernetes.Clientset, operatorConfig *config.Config) error {
	caKeypair, err := setupWebhookTLS(operatorConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to setup TLS for Admission Webhooks")
	}

	if err = setupValidatingWebhookConfig(kubeClient, operatorConfig, *caKeypair); err != nil {
		return errors.Wrapf(err, "failed to setup ValidatingWebhookConfiguration")
	}

	if err = setupWebhookService(kubeClient, operatorConfig); err != nil {
		return errors.Wrapf(err, "failed to setup Webhooks Service")
	}

	return nil
}

func DeleteWebhookAssets(kubeClient *kubernetes.Clientset, operatorConfig *config.Config) error {
	err := deleteWebhookService(kubeClient, operatorConfig)
	if err != nil {
		return errors.Wrap(err, "failed to delete webhook service")
	}

	err = deleteValidatingWebhookConfig(kubeClient)
	if err != nil {
		return errors.Wrap(err, "failed to delete validating webhook config")
	}

	return nil
}
