package webhooks

import (
	"fmt"
	"github.com/ibm/cassandra-operator/controllers/certs"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"path/filepath"
)

func setupWebhookTLS(operatorConfig *config.Config) (*certs.Keypair, error) {
	// Every time operator start we renew the certificates
	opts := certs.MakeDefaultOptions()
	opts.Org = "cassandra_operator"

	caKp, err := certs.CreateCA(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Webhook CA keypair")
	}

	opts.DnsNames = []string{
		names.WebhooksServiceName(),
		fmt.Sprintf("%s.%s", names.WebhooksServiceName(), operatorConfig.Namespace),
		fmt.Sprintf("%s.%s.svc", names.WebhooksServiceName(), operatorConfig.Namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", names.WebhooksServiceName(), operatorConfig.Namespace),
	}

	kp, err := certs.CreateCertificate(*caKp, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to crate Webhook keypair")
	}

	if err := writeWebhookTLS(names.OperatorWebhookTLSDir(), *kp); err != nil {
		return nil, errors.Wrap(err, "failed to write webhook TLS certificates")
	}

	return caKp, nil
}

func writeWebhookTLS(dir string, kp certs.Keypair) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Wrap(err, "failed to create certs dir")
	}

	fMode := os.FileMode(0600)

	if err := ioutil.WriteFile(filepath.Join(dir, "tls.crt"), kp.Crt, fMode); err != nil {
		return errors.Wrap(err, "failed to write Webhook TLS certificate to container's filesystem")
	}

	if err := ioutil.WriteFile(filepath.Join(dir, "tls.key"), kp.Pk, fMode); err != nil {
		return errors.Wrap(err, "failed to write Webhook TLS key to container's filesystem")
	}

	return nil
}
