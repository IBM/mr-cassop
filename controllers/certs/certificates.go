package certs

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/pkg/errors"
	"math/big"
	"time"
)

var ErrInvalidPEMBlock = errors.New("Invalid PEM block")

type Keypair interface {
	// Certificate returns the PEM-encoded x509 certificate
	Certificate() []byte
	// PrivateKey returns the PEM-encoded PKCS1 private key
	PrivateKey() []byte
}

type keypair struct {
	crt []byte
	pk  []byte
}

func (kp *keypair) Certificate() []byte {
	return kp.crt
}

func (kp *keypair) PrivateKey() []byte {
	return kp.pk
}

// CreateCA generates a new self-signed CA keypair
func CreateCA(options *CertOpts) (Keypair, error) {

	ca := &x509.Certificate{
		IsCA:                  true,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(options.Expire),
		SerialNumber:          options.SerialNum,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		Subject:               pkix.Name{Organization: []string{options.Org}},
	}

	pk, err := rsa.GenerateKey(rand.Reader, options.KeySize)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate private key for CA")
	}

	crtBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &pk.PublicKey, pk)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate certificate for CA")
	}

	pkBytes := x509.MarshalPKCS1PrivateKey(pk)

	return &keypair{
		crt: encodeToPemFormat("CERTIFICATE", crtBytes),
		pk:  encodeToPemFormat("RSA PRIVATE KEY", pkBytes),
	}, nil
}

// CreateCertificate generates a new keypair signed by self-signed CA
func CreateCertificate(caKp Keypair, options *CertOpts) (Keypair, error) {

	caCrt, err := parseCertificate(caKp.Certificate())
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse CA certificate")
	}

	caPk, err := parsePrivateKey(caKp.PrivateKey())
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse CA private key")
	}

	// Generate new KeyPair
	crt := &x509.Certificate{
		IsCA:                  false,
		NotBefore:             time.Now(),
		NotAfter:              caCrt.NotAfter,
		SerialNumber:          options.SerialNum,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		Subject:               caCrt.Subject,
		DNSNames:              options.DnsNames,
	}

	if len(options.DnsNames) > 0 {
		// CommonName must precisely match the server name where the certificate is installed and can only contain up to one entry.
		crt.Subject.CommonName = options.DnsNames[0]
	}

	pk, err := rsa.GenerateKey(rand.Reader, options.KeySize)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate private key")
	}
	pkBytes := x509.MarshalPKCS1PrivateKey(pk)

	// generate new keypair and sign with self-signed CA
	crtBytes, err := x509.CreateCertificate(rand.Reader, crt, caCrt, &pk.PublicKey, caPk)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create certificate")
	}

	return &keypair{
		crt: encodeToPemFormat("CERTIFICATE", crtBytes),
		pk:  encodeToPemFormat("RSA PRIVATE KEY", pkBytes),
	}, nil
}

// CertOpts contains options for generating keypair
type CertOpts struct {
	KeySize   int
	DnsNames  []string
	Expire    time.Duration
	Org       string
	SerialNum *big.Int
}

func MakeDefaultOptions() *CertOpts {
	return &CertOpts{
		KeySize:   4096,
		DnsNames:  []string{},
		Expire:    time.Hour * 24 * 365 * 10, // Set to 10 years
		Org:       "Self-Signed Issuer",
		SerialNum: big.NewInt(1),
	}
}

// encodeToPemFormat encodes to x509 PEM which represents Base64 encoded DER certificate
func encodeToPemFormat(kType string, data []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  kType,
		Bytes: data,
	})
}

func parseCertificate(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.Wrap(ErrInvalidPEMBlock, "failed to decode CA certificate")
	}

	caCrt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse certificate")
	}

	return caCrt, nil
}

func parsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.Wrap(ErrInvalidPEMBlock, "failed to decode private key")
	}

	caPk, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse private key")
	}
	return caPk, nil
}
