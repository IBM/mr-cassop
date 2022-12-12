package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/icarus"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("created cassandrarestore", func() {
	ccTpl := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(6),
				},
			},
			AdminRoleSecretName: "admin-role",
			ImagePullSecretName: "pullSecretName",
		},
	}

	storageSecretTpl := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "storage-credentials", Namespace: cassandraObjectMeta.Namespace},
		Data: map[string][]byte{
			"awsaccesskeyid":     []byte("key-id"),
			"awssecretaccesskey": []byte("access-key"),
			"awsregion":          []byte("us-east"),
			"awsendpoint":        []byte("https://s3.us-east.cloud-object-storage.appdomain.cloud"),
		},
	}

	cbTpl := &v1alpha1.CassandraBackup{
		ObjectMeta: cassandraBackupObjectMeta,
		Spec: v1alpha1.CassandraBackupSpec{
			CassandraCluster: cassandraObjectMeta.Name,
			StorageLocation:  "s3://bucket",
			SecretName:       storageSecretTpl.Name,
		},
	}

	crTpl := &v1alpha1.CassandraRestore{
		ObjectMeta: cassandraRestoreObjectMeta,
		Spec: v1alpha1.CassandraRestoreSpec{
			CassandraCluster: cassandraObjectMeta.Name,
			CassandraBackup:  cassandraBackupObjectMeta.Name,
		},
	}

	It("should send an icarus restore request and track progress", func() {
		cc := ccTpl.DeepCopy()
		cr := crTpl.DeepCopy()
		cb := cbTpl.DeepCopy()
		createReadyCluster(cc)
		Expect(k8sClient.Create(ctx, storageSecretTpl.DeepCopy())).To(Succeed())
		Expect(k8sClient.Create(ctx, cb)).To(Succeed())
		Eventually(func() []icarus.Backup {
			return mockIcarusClient.backups
		}, mediumTimeout, mediumRetry).Should(HaveLen(1))

		mockIcarusClient.backups[0].Progress = 1.0
		mockIcarusClient.backups[0].State = icarus.StateCompleted

		Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		Eventually(func() []icarus.Restore {
			return mockIcarusClient.restores
		}, mediumTimeout, mediumRetry).Should(HaveLen(1))
		Eventually(func() string {
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}, cr)).To(Succeed())
			return cr.Status.State
		}, mediumTimeout, mediumRetry).Should(Equal(icarus.StateRunning))
		Eventually(func() int {
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}, cr)).To(Succeed())
			return cr.Status.Progress
		}, mediumTimeout, mediumRetry).Should(Equal(0))

		mockIcarusClient.restores[0].Progress = 0.53

		Eventually(func() int {
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}, cr)).To(Succeed())
			return cr.Status.Progress
		}, mediumTimeout, mediumRetry).Should(Equal(53))

		mockIcarusClient.restores[0].Progress = 1
		mockIcarusClient.restores[0].State = icarus.StateCompleted

		Eventually(func() string {
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}, cr)).To(Succeed())
			return cr.Status.State
		}, mediumTimeout, mediumRetry).Should(Equal(icarus.StateCompleted))
		Eventually(func() int {
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}, cr)).To(Succeed())
			return cr.Status.Progress
		}, mediumTimeout, mediumRetry).Should(Equal(100))
	})

	Context("with failed restore", func() {
		It("should reflect errors in the status", func() {
			cc := ccTpl.DeepCopy()
			cr := crTpl.DeepCopy()
			cb := cbTpl.DeepCopy()
			createReadyCluster(cc)
			Expect(k8sClient.Create(ctx, storageSecretTpl.DeepCopy())).To(Succeed())
			Expect(k8sClient.Create(ctx, cb)).To(Succeed())
			Eventually(func() []icarus.Backup {
				return mockIcarusClient.backups
			}, mediumTimeout, mediumRetry).Should(HaveLen(1))

			mockIcarusClient.backups[0].Progress = 1.0
			mockIcarusClient.backups[0].State = icarus.StateCompleted

			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			Eventually(func() []icarus.Restore {
				return mockIcarusClient.restores
			}, mediumTimeout, mediumRetry).Should(HaveLen(1))
			Eventually(func() string {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}, cr)).To(Succeed())
				return cr.Status.State
			}, mediumTimeout, mediumRetry).Should(Equal(icarus.StateRunning))
			Eventually(func() int {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}, cr)).To(Succeed())
				return cr.Status.Progress
			}, mediumTimeout, mediumRetry).Should(Equal(0))

			mockIcarusClient.restores[0].Progress = 1
			mockIcarusClient.restores[0].State = icarus.StateFailed
			mockIcarusClient.restores[0].Errors = []icarus.Error{
				{
					Source:  "pod-1",
					Message: "some error",
				},
				{
					Source:  "pod-2",
					Message: "another error",
				},
			}

			Eventually(func() string {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}, cr)).To(Succeed())
				return cr.Status.State
			}, mediumTimeout, mediumRetry).Should(Equal(icarus.StateFailed))
			Eventually(func() int {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}, cr)).To(Succeed())
				return cr.Status.Progress
			}, mediumTimeout, mediumRetry).Should(Equal(100))
			Eventually(func() []v1alpha1.RestoreError {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}, cr)).To(Succeed())
				return cr.Status.Errors
			}, mediumTimeout, mediumRetry).Should(BeEquivalentTo([]v1alpha1.RestoreError{
				{
					Source:  "pod-1",
					Message: "some error",
				},
				{
					Source:  "pod-2",
					Message: "another error",
				},
			}))
		})
	})
})
