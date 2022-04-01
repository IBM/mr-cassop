package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/certs"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type tlsSecretType int

const (
	clientCA = iota
	clientNode
	serverCA
	serverNode
)

type tlsSecret struct {
	secretName  *string
	defaultName string
	fieldPath   string
	annotation  string
	secretType  tlsSecretType
}

func (r *CassandraClusterReconciler) reconcileTLSSecrets(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if err := r.reconcileServerEncryption(ctx, cc); err != nil {
		return err
	}
	if err := r.reconcileClientEncryption(ctx, cc); err != nil {
		return err
	}
	return nil
}

func newTLSSecret(cc *dbv1alpha1.CassandraCluster, secretType tlsSecretType) *tlsSecret {
	switch secretType {
	case clientCA:
		return &tlsSecret{
			&cc.Spec.Encryption.Client.CATLSSecret.Name,
			names.CassandraClientTLSCA(cc.Name),
			"encryption.client.caTLSSecret.name",
			"client-ca-tls-secret",
			clientCA,
		}
	case clientNode:
		return &tlsSecret{
			&cc.Spec.Encryption.Client.NodeTLSSecret.Name,
			names.CassandraClientTLSNode(cc.Name),
			"encryption.client.nodeTLSSecret.name",
			"client-node-tls-secret",
			clientNode,
		}
	case serverCA:
		return &tlsSecret{
			&cc.Spec.Encryption.Server.CATLSSecret.Name,
			names.CassandraClusterTLSCA(cc.Name),
			"encryption.server.caTLSSecret.name",
			"cluster-ca-tls-secret",
			serverCA,
		}
	case serverNode:
		return &tlsSecret{
			&cc.Spec.Encryption.Server.NodeTLSSecret.Name,
			names.CassandraClusterTLSNode(cc.Name),
			"encryption.server.nodeTLSSecret.name",
			"cluster-node-tls-secret",
			serverNode,
		}
	default:
		log.Panicf("invalid TLS secret type: %v", secretType)
	}
	return nil
}

func newTLSSecretRequiredFields(cc *dbv1alpha1.CassandraCluster, secretType tlsSecretType) ([]string, error) {
	var tlsSecret dbv1alpha1.NodeTLSSecret
	switch secretType {
	case clientNode:
		tlsSecret = cc.Spec.Encryption.Client.NodeTLSSecret
	case serverNode:
		tlsSecret = cc.Spec.Encryption.Server.NodeTLSSecret
	default:
		return nil, errors.New(fmt.Sprintf("only node secrets should check required fields. secretType: %v", secretType))
	}
	return []string{
		tlsSecret.CACrtFileKey,
		tlsSecret.CrtFileKey,
		tlsSecret.FileKey,
		tlsSecret.KeystoreFileKey,
		tlsSecret.TruststoreFileKey,
		tlsSecret.KeystorePasswordKey,
		tlsSecret.TruststorePasswordKey,
	}, nil
}

func (r *CassandraClusterReconciler) validateTLSFields(cc *dbv1alpha1.CassandraCluster, tlsSecret *v1.Secret, secretType tlsSecretType) error {
	requiredFields, err := newTLSSecretRequiredFields(cc, secretType)
	if err != nil {
		return err
	}
	emptyFields := util.EmptySecretFields(tlsSecret, requiredFields)
	if tlsSecret.Data == nil || len(emptyFields) != 0 {
		errMsg := fmt.Sprintf("TLS Secret `%s` has some empty or missing fields: %v", tlsSecret.Name, emptyFields)
		r.Log.Warnf(errMsg)
		r.Events.Warning(cc, events.EventTLSSecretInvalid, errMsg)
		return errors.Errorf(errMsg)
	}
	return nil
}

func (r *CassandraClusterReconciler) reconcileNodeTLSSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster, restartChecksum checksumContainer, secretType tlsSecretType) (*v1.Secret, error) {
	secret := newTLSSecret(cc, secretType)
	tlsSecret, err := r.getTLSSecret(ctx, cc, secret, false)
	if err != nil {
		return nil, err
	}

	if err = r.validateTLSFields(cc, tlsSecret, secretType); err != nil {
		return nil, errors.Wrapf(err, "failed to validate %s: %s fields", secret.fieldPath, *secret.secretName)
	}

	annotations := make(map[string]string)
	annotations[dbv1alpha1.CassandraClusterInstance] = cc.Name
	err = r.reconcileAnnotations(ctx, tlsSecret, annotations)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to reconcile annotations for secret `%s`", tlsSecret.Name)
	}

	restartChecksum[secret.annotation] = fmt.Sprintf("%v", tlsSecret.Data)
	return tlsSecret, nil
}

func (r *CassandraClusterReconciler) getTLSSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster, secret *tlsSecret, allowDefaulting bool) (*v1.Secret, error) {
	tlsSecret := &v1.Secret{}
	secretName := secret.secretName
	err := r.Get(ctx, types.NamespacedName{Name: *secretName, Namespace: cc.Namespace}, tlsSecret)
	if err != nil && kerrors.IsNotFound(err) {
		errMsg := fmt.Sprintf("%s: %s was not found", secret.fieldPath, *secretName)
		if allowDefaulting {
			errMsg = fmt.Sprintf("%s. falling back to default secret, %s, and generating it", errMsg, secret.defaultName)
			*secretName = secret.defaultName
		}
		r.Events.Warning(cc, events.EventTLSSecretNotFound, errMsg)
	} else if err != nil {
		return nil, errors.Wrapf(err, "failed to get secret: %s: %s", secret.fieldPath, *secretName)
	}
	return tlsSecret, nil
}

func (r *CassandraClusterReconciler) reconcileServerEncryption(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if cc.Spec.Encryption.Server.InternodeEncryption == dbv1alpha1.InternodeEncryptionNone {
		return nil
	}

	if cc.Spec.Encryption.Server.CATLSSecret.Name == "" {
		// Use default CA TLS Secret name
		cc.Spec.Encryption.Server.CATLSSecret.Name = names.CassandraClusterTLSCA(cc.Name)
	} else if _, err := r.getTLSSecret(ctx, cc, newTLSSecret(cc, serverCA), true); err != nil {
		return err
	}

	if cc.Spec.Encryption.Server.NodeTLSSecret.Name == "" {
		// Generate TLS CA Secret bc user didn't set Node TLS Secret name
		err := r.handleCASecret(ctx, cc, cc.Spec.Encryption.Server.CATLSSecret.Name)
		if err != nil {
			return errors.Wrapf(err, "Failed to handle Cluster TLS CA Secret")
		}
	}

	if cc.Spec.Encryption.Server.NodeTLSSecret.Name == "" {
		// Use default Node TLS Secret name
		cc.Spec.Encryption.Server.NodeTLSSecret.Name = names.CassandraClusterTLSNode(cc.Name)
	} else if _, err := r.getTLSSecret(ctx, cc, newTLSSecret(cc, serverNode), true); err != nil {
		return err
	}

	// Node TLS Secret will be generated if not exist
	err := r.handleNodeSecret(ctx, cc, cc.Spec.Encryption.Server.CATLSSecret.Name, cc.Spec.Encryption.Server.NodeTLSSecret)
	if err != nil {
		return errors.Wrapf(err, "Failed to handle Cluster TLS Node Secret")
	}
	return nil
}

func (r *CassandraClusterReconciler) reconcileClientEncryption(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if !cc.Spec.Encryption.Client.Enabled {
		return nil
	}

	if cc.Spec.Encryption.Client.CATLSSecret.Name == "" {
		// Use default CA TLS Secret name
		cc.Spec.Encryption.Client.CATLSSecret.Name = names.CassandraClientTLSCA(cc.Name)
	} else if _, err := r.getTLSSecret(ctx, cc, newTLSSecret(cc, clientCA), true); err != nil {
		return err
	}

	if cc.Spec.Encryption.Client.NodeTLSSecret.Name == "" {
		// Generate TLS CA Secret bc user didn't set Node TLS Secret name
		err := r.handleCASecret(ctx, cc, cc.Spec.Encryption.Client.CATLSSecret.Name)
		if err != nil {
			return errors.Wrapf(err, "Failed to handle Client TLS CA Secret")
		}
	}

	if cc.Spec.Encryption.Client.NodeTLSSecret.Name == "" {
		// Use default Node TLS Secret name
		cc.Spec.Encryption.Client.NodeTLSSecret.Name = names.CassandraClientTLSNode(cc.Name)
	} else if _, err := r.getTLSSecret(ctx, cc, newTLSSecret(cc, clientNode), true); err != nil {
		return err
	}

	// Node TLS Secret will be generated if not exist
	err := r.handleNodeSecret(ctx, cc, cc.Spec.Encryption.Client.CATLSSecret.Name, cc.Spec.Encryption.Client.NodeTLSSecret)
	if err != nil {
		return errors.Wrapf(err, "Failed to handle Client TLS Node Secret")
	}

	err = r.setupClientTLSFiles(ctx, cc)
	if err != nil {
		return errors.Wrap(err, "failed to obtain Client TLS")
	}

	return nil
}

func (r *CassandraClusterReconciler) handleCASecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster, casSecretName string) error {
	actualTLSCA := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: casSecretName}, actualTLSCA)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Infof("TLS CA Secret `%s` not found. Generating it...", casSecretName)

		desiredTLSCA, err := genCASecret(cc, casSecretName)
		if err != nil {
			return errors.Wrapf(err, "Unable to handle TLS CA Secret `%s`", casSecretName)
		}

		if err = controllerutil.SetControllerReference(cc, desiredTLSCA, r.Scheme); err != nil {
			return errors.Wrap(err, "Cannot set controller reference")
		}

		if err = r.Create(ctx, desiredTLSCA); err != nil {
			return errors.Wrapf(err, "Unable to create Secret `%s`", desiredTLSCA.Name)
		}

	} else if err != nil {
		return errors.Wrapf(err, "Failed to get TLS CA Secret `%s`", casSecretName)
	}
	r.Log.Debugf("TLS CA Secret was found `%s`", casSecretName)

	return nil
}

func (r *CassandraClusterReconciler) handleNodeSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster, casSecretName string, nodeTLSSecret dbv1alpha1.NodeTLSSecret) error {
	actualTLSNode := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: nodeTLSSecret.Name}, actualTLSNode)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Infof("TLS Node Secret `%s` not found. Generating it...", nodeTLSSecret.Name)
		r.Log.Infof("Reading data from TLS CA Secret `%s`...", casSecretName)
		actualTLSCA := &v1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: casSecretName}, actualTLSCA)
		if err != nil && kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "TLS CA Secret `%s` not found", casSecretName)
		} else if err != nil {
			return errors.Wrapf(err, "Failed to get TLS CA Secret `%s`", casSecretName)
		}

		caKp := certs.Keypair{
			Crt: actualTLSCA.Data["ca.crt"],
			Pk:  actualTLSCA.Data["ca.key"],
		}

		opts := certs.MakeDefaultOptions()
		desiredTLSNodeSecret, err := genNodeSecret(caKp, opts, cc, nodeTLSSecret)
		if err != nil {
			return errors.Wrapf(err, "Failed to generate TLS Node Secret")
		}

		// Steps to create Node TLS Secret
		if err = controllerutil.SetControllerReference(cc, desiredTLSNodeSecret, r.Scheme); err != nil {
			return errors.Wrap(err, "Cannot set controller reference")
		}

		r.Log.Infof("Creating TLS Node Secret `%s`...", nodeTLSSecret.Name)
		if err = r.Create(ctx, desiredTLSNodeSecret); err != nil {
			return errors.Wrapf(err, "Failed to create TLS Node Secret")
		}

		return nil

	} else if err != nil {
		return errors.Wrapf(err, "Failed to get TLS Node Secret `%s`", nodeTLSSecret.Name)
	}

	r.Log.Debugf("TLS Node Secret was found `%s`", nodeTLSSecret.Name)

	return nil
}

func genCASecret(cc *dbv1alpha1.CassandraCluster, caSecretName string) (*v1.Secret, error) {
	opts := certs.MakeDefaultOptions()
	opts.Org = "Cassandra Operator"

	caKp, err := certs.CreateCA(opts)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate CA keypair")
	}

	desiredTLSCA := &v1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      caSecretName,
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
		},
		Type: v1.SecretTypeOpaque,
	}

	data := make(map[string][]byte)
	data["ca.crt"] = caKp.Crt
	data["ca.key"] = caKp.Pk
	desiredTLSCA.Data = data

	return desiredTLSCA, nil
}

func genNodeSecret(caKp certs.Keypair, opts *certs.CertOpts, cc *dbv1alpha1.CassandraCluster, nodeTLSSecret dbv1alpha1.NodeTLSSecret) (*v1.Secret, error) {
	desiredTLSNodeSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeTLSSecret.Name,
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
		},
		Type: v1.SecretTypeOpaque,
	}

	opts.DnsNames = []string{"localhost"}
	kp, err := certs.CreateCertificate(caKp, opts)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create TLS Node keypair")
	}

	parsedCACrt, err := certs.ParseCertificate(caKp.Crt)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot parse CA certificate")
	}

	parsedCrt, err := certs.ParseCertificate(kp.Crt)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot parse certificate")
	}

	parsedKey, err := certs.ParsePrivateKey(kp.Pk)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot parse private key")
	}

	keystorePFXBytes, err := certs.GeneratePFXKeystore(parsedKey, parsedCrt, parsedCACrt, nodeTLSSecret.GenerateKeystorePassword)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot create PFX keystore")
	}

	truststorePFXBytes, err := certs.GeneratePFXTruststore(parsedCACrt, nodeTLSSecret.GenerateKeystorePassword)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot create PFX trustore")
	}

	data := make(map[string][]byte)
	data[nodeTLSSecret.CACrtFileKey] = caKp.Crt
	data[nodeTLSSecret.CrtFileKey] = kp.Crt
	data[nodeTLSSecret.FileKey] = kp.Pk
	data[nodeTLSSecret.KeystoreFileKey] = keystorePFXBytes
	data[nodeTLSSecret.KeystorePasswordKey] = []byte(nodeTLSSecret.GenerateKeystorePassword)
	data[nodeTLSSecret.TruststoreFileKey] = truststorePFXBytes
	data[nodeTLSSecret.TruststorePasswordKey] = []byte(nodeTLSSecret.GenerateKeystorePassword)
	desiredTLSNodeSecret.Data = data

	return desiredTLSNodeSecret, nil
}

func (r *CassandraClusterReconciler) setupClientTLSFiles(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	clientTLSSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: cc.Spec.Encryption.Client.NodeTLSSecret.Name}, clientTLSSecret)
	if err != nil {
		return errors.Wrapf(err, "failed to get Client TLS Secret: %s", cc.Spec.Encryption.Client.NodeTLSSecret.Name)
	}

	err = os.MkdirAll(names.OperatorClientTLSDir(cc), 0700)
	if err != nil {
		return errors.Wrapf(err, "failed to create directory: %s", names.OperatorClientTLSDir(cc))
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.NodeTLSSecret.CACrtFileKey), clientTLSSecret.Data[cc.Spec.Encryption.Client.NodeTLSSecret.CACrtFileKey], 0600)
	if err != nil {
		return errors.Wrapf(err, "failed to write CA certificate into file %s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.NodeTLSSecret.CACrtFileKey)
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.NodeTLSSecret.FileKey), clientTLSSecret.Data[cc.Spec.Encryption.Client.NodeTLSSecret.FileKey], 0600)
	if err != nil {
		return errors.Wrapf(err, "failed to write private key into file %s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.NodeTLSSecret.FileKey)
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.NodeTLSSecret.CrtFileKey), clientTLSSecret.Data[cc.Spec.Encryption.Client.NodeTLSSecret.CrtFileKey], 0600)
	if err != nil {
		return errors.Wrapf(err, "failed to write certificate into file %s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.NodeTLSSecret.CrtFileKey)
	}

	return nil
}

func (r *CassandraClusterReconciler) cleanupClientTLSDir(cc *dbv1alpha1.CassandraCluster) {
	err := os.RemoveAll(names.OperatorClientTLSDir(cc))
	if err != nil {
		r.Log.Errorf("%+v", err)
	}
}
