package controllers

import (
	"github.com/gogo/protobuf/proto"
	"github.com/golang/mock/gomock"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestReconcileRFSettings(t *testing.T) {
	asserts := NewGomegaWithT(t)
	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			},
			SystemKeyspaces: v1alpha1.SystemKeyspaces{
				Names: []v1alpha1.KeyspaceName{"system_auth"},
				DCs: []v1alpha1.SystemKeyspaceDC{
					{
						Name: "dc1",
						RF:   3,
					},
				},
			},
		},
	}

	adminSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ActiveAdminSecret(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
		Immutable: proto.Bool(true),
		Type:      v1.SecretTypeOpaque,
	}

	data := make(map[string][]byte)
	cassandraOperatorAdminPassword := util.GenerateAdminPassword()
	data[v1alpha1.CassandraOperatorAdminRole] = []byte(cassandraOperatorAdminPassword)

	adminSecret.Data = data

	t.Run("should be no error if no keyspace specified", func(t *testing.T) {
		ccWithEmptySystemKeyspaces := &v1alpha1.CassandraCluster{
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
		reconciler, mCtrl, mocks := createMockedReconciler(t)
		mocks.cql.EXPECT().GetKeyspacesInfo().Times(1).Return([]cql.Keyspace{{Name: "system_auth", Replication: map[string]string{"class": "smth"}}}, nil)
		mocks.cql.EXPECT().UpdateRF("system_auth", map[string]string{"class": cql.ReplicationClassNetworkTopologyStrategy, "dc1": "3"}).Times(1).Return(nil)
		mocks.nodetool.EXPECT().RepairKeyspace(ccWithEmptySystemKeyspaces, "system_auth").Times(1).Return(nil)
		err := reconciler.reconcileKeyspaces(ccWithEmptySystemKeyspaces, mocks.cql, mocks.nodetool)

		asserts.Expect(err).To(BeNil())
		mCtrl.Finish()
	})

	t.Run("return error if can't get keyspace info", func(t *testing.T) {
		reconciler, mCtrl, mocks := createMockedReconciler(t)
		mocks.cql.EXPECT().GetKeyspacesInfo().Times(1).Return([]cql.Keyspace{}, errors.New("query error"))
		err := reconciler.reconcileKeyspaces(cc, mocks.cql, mocks.nodetool)
		asserts.Expect(err).ToNot(BeNil())
		mCtrl.Finish()
	})

	t.Run("happy path", func(t *testing.T) {
		reconciler, mCtrl, mocks := createMockedReconciler(t)
		mocks.cql.EXPECT().GetKeyspacesInfo().Times(1).Return([]cql.Keyspace{{
			Name: "system_auth",
			Replication: map[string]string{
				"class": "org.apache.cassandra.locator.SimpleTopologyStrategy",
			},
		}}, nil)

		mocks.cql.EXPECT().UpdateRF("system_auth", map[string]string{
			"class": "org.apache.cassandra.locator.NetworkTopologyStrategy",
			"dc1":   "3",
		}).Times(1).Return(nil)
		mocks.nodetool.EXPECT().RepairKeyspace(cc, "system_auth").Times(1).Return(nil)
		err := reconciler.reconcileKeyspaces(cc, mocks.cql, mocks.nodetool)
		asserts.Expect(err).To(BeNil())
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
		mocks.cql.EXPECT().UpdateRF("system_auth", gomock.Any()).Times(1).Return(nil)
		mocks.nodetool.EXPECT().RepairKeyspace(cc, "system_auth").Times(1).Return(errors.New("err while repair"))

		err := reconciler.reconcileKeyspaces(cc, mocks.cql, mocks.nodetool)
		asserts.Expect(err).ToNot(BeNil())
		mCtrl.Finish()
	})
}
