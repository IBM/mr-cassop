package certs

import (
	"crypto/rand"
	"crypto/x509"
	"github.com/pkg/errors"
	"software.sslmate.com/src/go-pkcs12"
)

func GeneratePFXKeystore(privateKey interface{}, certificate *x509.Certificate, caCertificates []*x509.Certificate, keystorePassword string) ([]byte, error) {
	keystorePFXBytes, err := pkcs12.Encode(rand.Reader, privateKey, certificate, caCertificates, keystorePassword)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot encode PFX Keystore")
	}

	_, _, _, err = pkcs12.DecodeChain(keystorePFXBytes, keystorePassword)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot validate PFX Keystore")
	}

	return keystorePFXBytes, nil
}

func GeneratePFXTruststore(caCertificates []*x509.Certificate, keystorePassword string) ([]byte, error) {
	truststorePFXBytes, err := pkcs12.EncodeTrustStore(rand.Reader, caCertificates, keystorePassword)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot encode PFX Truststore")
	}

	_, err = pkcs12.DecodeTrustStore(truststorePFXBytes, keystorePassword)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot validate PFX Truststore")
	}

	return truststorePFXBytes, nil
}
