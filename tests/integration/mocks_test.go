package integration

import (
	"context"
	"fmt"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
)

type proberMock struct {
	readyAllDCs bool
	ready       bool
	err         error
}

func (r proberMock) Ready(ctx context.Context) (bool, error) {
	return r.ready, r.err
}

func (r proberMock) ReadyAllDCs(ctx context.Context) (bool, error) {
	return r.readyAllDCs, r.err
}

type cqlMock struct {
	err            error
	keyspaces      []cql.Keyspace
	cassandraUsers []cql.CassandraUser
}

func (c *cqlMock) Query(stmt string, values ...interface{}) error {
	return c.err
}

func (c *cqlMock) GetKeyspacesInfo() ([]cql.Keyspace, error) {
	return c.keyspaces, c.err
}

func (c *cqlMock) GetUsers() ([]cql.CassandraUser, error) {
	return c.cassandraUsers, c.err
}

func (c *cqlMock) UpdateRF(cc *dbv1alpha1.CassandraCluster) error {
	var systemAuthIndex *int
	for i, keyspace := range c.keyspaces {
		if keyspace.Name == "system_auth" {
			index := i
			systemAuthIndex = &index
			break
		}
	}

	var systemAuth cql.Keyspace
	rfs := map[string]string{}
	for _, dc := range cc.Spec.DCs {
		rfs[dc.Name] = fmt.Sprintf("%d", *dc.Replicas)
	}

	rfs["class"] = "org.apache.cassandra.locator.NetworkTopologyStrategy"

	systemAuth.Replication = rfs
	systemAuth.Name = "system_auth"

	if systemAuthIndex != nil {
		c.keyspaces[*systemAuthIndex] = systemAuth
	} else {
		c.keyspaces = append(c.keyspaces, systemAuth)
	}

	return c.err
}

type nodetoolMock struct {
	err error
}

func (n *nodetoolMock) RepairKeyspace(cc *dbv1alpha1.CassandraCluster, keyspace string) error {
	return n.err
}
