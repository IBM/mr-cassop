package prober

import (
	"testing"

	"github.com/onsi/gomega"
	"go.uber.org/zap"

	v1 "k8s.io/api/core/v1"
)

func TestHandleAddSecret(t *testing.T) {
	asserts := gomega.NewWithT(t)
	initialRoleName := "testRole"
	initialRolePassword := "testPassword"
	testProber := &Prober{
		auth: UserAuth{
			User:     initialRoleName,
			Password: initialRolePassword,
		},
		log: zap.NewNop().Sugar(),
		jolokia: &jolokiaMock{
			username: initialRoleName,
			password: initialRolePassword,
		},
	}

	newRole := "newRole"
	newPassword := "newPassword"
	secret := &v1.Secret{
		Data: map[string][]byte{
			"admin-role":     []byte(newRole),
			"admin-password": []byte(newPassword),
		},
	}

	testProber.handleAddAuthSecret(secret)

	asserts.Expect(testProber.auth.User).To(gomega.Equal(newRole))
	asserts.Expect(testProber.auth.Password).To(gomega.Equal(newPassword))

	mockJolokia, ok := testProber.jolokia.(*jolokiaMock)
	asserts.Expect(ok).To(gomega.BeTrue())
	asserts.Expect(mockJolokia.username).To(gomega.Equal(newRole))
	asserts.Expect(mockJolokia.password).To(gomega.Equal(newPassword))
}
