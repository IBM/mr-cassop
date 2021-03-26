package integration

import (
	"context"
	"github.com/gocql/gocql"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
)

type proberMock struct {
	ready bool
	seeds []string
	err   error
}

type cqlMock struct {
	keyspaces      []cql.Keyspace
	cassandraRoles []cql.Role
	err            error
}

type nodetoolMock struct {
	err error
}

type reaperMock struct {
	repairs       []dbv1alpha1.Repair
	isRunning     bool
	clusterExists bool
	err           error
}

func (r proberMock) Ready(ctx context.Context) (bool, error) {
	return r.ready, r.err
}

func (r proberMock) Seeds(ctx context.Context) ([]string, error) {
	return r.seeds, r.err
}

func (c *cqlMock) Query(stmt string, values ...interface{}) error {
	return c.err
}

func (c *cqlMock) GetKeyspacesInfo() ([]cql.Keyspace, error) {
	return c.keyspaces, c.err
}

func (c *cqlMock) GetRoles() ([]cql.Role, error) {
	return c.cassandraRoles, c.err
}

func (c *cqlMock) UpdateRole(role cql.Role) error {
	for i, cassandraRole := range c.cassandraRoles {
		if cassandraRole.Role == role.Role {
			c.cassandraRoles[i] = role
			return nil
		}
	}

	return gocql.ErrNotFound
}

func (c *cqlMock) CreateRole(role cql.Role) error {
	for _, cassandraRole := range c.cassandraRoles {
		if cassandraRole.Role == role.Role {
			return errors.New("role already exists")
		}
	}

	c.cassandraRoles = append(c.cassandraRoles, role)
	return c.err
}

func (c *cqlMock) UpdateRF(keyspaceName string, rfOptions map[string]string) error {
	var keyspaceIndex *int
	for i, keyspace := range c.keyspaces {
		if keyspace.Name == keyspaceName {
			index := i
			keyspaceIndex = &index
			break
		}
	}

	if keyspaceIndex == nil {
		return gocql.ErrKeyspaceDoesNotExist
	}

	var keyspace cql.Keyspace
	keyspace.Replication = rfOptions
	keyspace.Name = keyspaceName

	c.keyspaces[*keyspaceIndex] = keyspace

	return c.err
}

func (n *nodetoolMock) RepairKeyspace(cc *dbv1alpha1.CassandraCluster, keyspace string) error {
	return n.err
}

func (r *reaperMock) IsRunning(ctx context.Context) (bool, error) {
	return r.isRunning, r.err
}
func (r *reaperMock) ClusterExists(ctx context.Context, name string) (bool, error) {
	return r.clusterExists, r.err
}
func (r *reaperMock) AddCluster(ctx context.Context, name, seed string) error {
	if !r.clusterExists {
		r.clusterExists = true
	}
	return r.err
}
func (r *reaperMock) ScheduleRepair(ctx context.Context, clusterName string, repair dbv1alpha1.Repair) error {
	r.repairs = append(r.repairs, repair)
	return r.err
}

func initializeReadyCluster(cc *dbv1alpha1.CassandraCluster) error {
	mockProberClient.err = nil
	mockProberClient.ready = true
	mockNodetoolClient.err = nil
	mockReaperClient.err = nil
	mockReaperClient.isRunning = false
	mockReaperClient.clusterExists = true
	mockCQLClient.err = nil
	mockCQLClient.cassandraRoles = []cql.Role{{Role: "cassandra", Super: true}}
	mockCQLClient.keyspaces = []cql.Keyspace{{
		Name: "system_auth",
		Replication: map[string]string{
			"class": "org.apache.cassandra.locator.SimpleTopologyStrategy",
		},
	}}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc); err != nil {
		return err
	}
	cc.Status.ReadyAllDCs = true
	return k8sClient.Status().Update(ctx, cc)
}
