package controllers

import (
	"errors"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestReconcileRFSettings(t *testing.T) {
	asserts := NewGomegaWithT(t)
	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.CassandraClusterSpec{
			SystemKeyspaces: v1alpha1.SystemKeyspaces{
				DCs: []v1alpha1.SystemKeyspaceDC{
					{
						Name: "dc1",
						RF:   3,
					},
				},
			},
		},
	}

	t.Run("rf settings are already correct", func(t *testing.T) {
		reconciler, mCtrl, mocks := createMockedReconciler(t)
		mocks.cql.EXPECT().GetKeyspacesInfo().Times(1).Return([]cql.Keyspace{{
			Name: "system_auth",
			Replication: map[string]string{
				"class": "org.apache.cassandra.locator.NetworkTopologyStrategy",
				"dc1":   "3",
			},
		}}, nil)
		err := reconciler.reconcileRFSettings(cc, mocks.cql, mocks.nodetool)
		asserts.Expect(err).To(BeNil())
		mCtrl.Finish()
	})

	t.Run("rf gets updated if not correct", func(t *testing.T) {
		reconciler, mCtrl, mocks := createMockedReconciler(t)
		mocks.cql.EXPECT().GetKeyspacesInfo().Times(1).Return([]cql.Keyspace{{
			Name: "system_auth",
			Replication: map[string]string{
				"class": "org.apache.cassandra.locator.NetworkTopologyStrategy",
				"dc1":   "2",
			},
		}}, nil)

		mocks.cql.EXPECT().UpdateRF(cc).Times(1).Return(nil)
		mocks.nodetool.EXPECT().RepairKeyspace(cc, "system_auth").Times(1).Return(nil)
		err := reconciler.reconcileRFSettings(cc, mocks.cql, mocks.nodetool)
		asserts.Expect(err).To(BeNil())
		mCtrl.Finish()
	})

	t.Run("fail if system_auth keyspace not present", func(t *testing.T) {
		reconciler, mCtrl, mocks := createMockedReconciler(t)
		mocks.cql.EXPECT().GetKeyspacesInfo().Times(1).Return([]cql.Keyspace{}, nil)
		err := reconciler.reconcileRFSettings(cc, mocks.cql, mocks.nodetool)
		asserts.Expect(err).ToNot(BeNil())
		mCtrl.Finish()
	})

	t.Run("fail if update rf query fails", func(t *testing.T) {
		reconciler, mCtrl, mocks := createMockedReconciler(t)
		mocks.cql.EXPECT().GetKeyspacesInfo().Times(1).Return([]cql.Keyspace{{
			Name: "system_auth",
			Replication: map[string]string{
				"class": "org.apache.cassandra.locator.NetworkTopologyStrategy",
				"dc1":   "2",
			},
		}}, nil)
		mocks.cql.EXPECT().UpdateRF(cc).Times(1).Return(errors.New("err while updating rf"))

		err := reconciler.reconcileRFSettings(cc, mocks.cql, mocks.nodetool)
		asserts.Expect(err).ToNot(BeNil())
		mCtrl.Finish()
	})

	t.Run("fail if can't repair keyspace", func(t *testing.T) {
		reconciler, mCtrl, mocks := createMockedReconciler(t)
		mocks.cql.EXPECT().GetKeyspacesInfo().Times(1).Return([]cql.Keyspace{{
			Name: "system_auth",
			Replication: map[string]string{
				"class": "org.apache.cassandra.locator.NetworkTopologyStrategy",
				"dc1":   "2",
			},
		}}, nil)
		mocks.cql.EXPECT().UpdateRF(cc).Times(1).Return(nil)
		mocks.nodetool.EXPECT().RepairKeyspace(cc, "system_auth").Times(1).Return(errors.New("err while repair"))

		err := reconciler.reconcileRFSettings(cc, mocks.cql, mocks.nodetool)
		asserts.Expect(err).ToNot(BeNil())
		mCtrl.Finish()
	})

	t.Run("fail if can't get keyspaces info", func(t *testing.T) {
		reconciler, mCtrl, mocks := createMockedReconciler(t)
		mocks.cql.EXPECT().GetKeyspacesInfo().Times(1).Return([]cql.Keyspace{}, errors.New("can't get keyspaces info"))
		err := reconciler.reconcileRFSettings(cc, mocks.cql, mocks.nodetool)
		asserts.Expect(err).ToNot(BeNil())
		mCtrl.Finish()
	})
}
