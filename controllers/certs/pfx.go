package certs

import (
	"crypto/rand"
	"crypto/x509"
	"github.com/pkg/errors"
	"software.sslmate.com/src/go-pkcs12"
)

func GeneratePFXKeystore(privateKey interface{}, certificate *x509.Certificate, caCertificate *x509.Certificate, keystorePassword string) ([]byte, error) {
	keystorePFXBytes, err := pkcs12.Encode(rand.Reader, privateKey, certificate, []*x509.Certificate{caCertificate}, keystorePassword)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot encode PFX Keystore")
	}

	_, _, _, err = pkcs12.DecodeChain(keystorePFXBytes, keystorePassword)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot validate PFX Keystore")
	}

	return keystorePFXBytes, nil
}

func GeneratePFXTruststore(certificate *x509.Certificate, keystorePassword string) ([]byte, error) {
	truststorePFXBytes, err := pkcs12.EncodeTrustStore(rand.Reader, []*x509.Certificate{certificate}, keystorePassword)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot encode PFX Truststore")
	}

	_, err = pkcs12.DecodeTrustStore(truststorePFXBytes, keystorePassword)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot validate PFX Truststore")
	}

	return truststorePFXBytes, nil
}
