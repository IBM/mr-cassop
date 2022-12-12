package certs

import (
	. "github.com/onsi/gomega"

	"testing"
)

func TestCreateCAandCertificate(t *testing.T) {
	asserts := NewWithT(t)
	opts := MakeDefaultOptions()
	caKeypair, err := CreateCA(opts)

	asserts.Expect(err).ToNot(HaveOccurred())
	asserts.Expect(caKeypair).To(BeAssignableToTypeOf(&Keypair{}))
	asserts.Expect(caKeypair).ToNot(BeNil())

	certKeypair, err := CreateCertificate(*caKeypair, opts)
	asserts.Expect(err).ToNot(HaveOccurred())
	asserts.Expect(certKeypair).To(BeAssignableToTypeOf(&Keypair{}))
	asserts.Expect(certKeypair).ToNot(BeNil())
}
