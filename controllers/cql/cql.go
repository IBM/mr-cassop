package cql

import (
	"fmt"
	"github.com/gocql/gocql"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/pkg/errors"
)

const (
	ReplicationClassNetworkTopologyStrategy = "org.apache.cassandra.locator.NetworkTopologyStrategy"
)

type CqlClient interface {
	GetKeyspacesInfo() ([]Keyspace, error)
	UpdateRF(cc *v1alpha1.CassandraCluster) error
	GetUsers() ([]CassandraUser, error)
	Query(stmt string, values ...interface{}) error
}

type cassandraClient struct {
	*gocql.Session
}

type CassandraUser struct {
	Role        string
	IsSuperuser bool
}

func NewCQLClient(clusterConfig *gocql.ClusterConfig) (CqlClient, error) {
	cassSession, err := clusterConfig.CreateSession()
	if err != nil {
		return nil, err
	}

	return &cassandraClient{Session: cassSession}, nil
}

type Keyspace struct {
	Name        string
	Replication map[string]string
}

func (c cassandraClient) Query(stmt string, values ...interface{}) error {
	return c.Session.Query(stmt, values).Exec()
}

func (c cassandraClient) GetKeyspacesInfo() ([]Keyspace, error) {
	iter := c.Session.Query("SELECT keyspace_name,replication FROM system_schema.keyspaces").Iter()
	var keyspaceName string
	replication := make(map[string]string)
	keyspaces := make([]Keyspace, 0, iter.NumRows())
	for iter.Scan(&keyspaceName, &replication) {
		keyspace := Keyspace{Name: keyspaceName, Replication: replication}

		keyspaces = append(keyspaces, keyspace)
	}

	err := iter.Close()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to close iterator")
	}
	return keyspaces, nil
}

func (c cassandraClient) UpdateRF(cc *v1alpha1.CassandraCluster) error {
	queryDCs := ""
	for _, dc := range cc.Spec.SystemKeyspaces.DCs {
		if queryDCs != "" {
			queryDCs = queryDCs + ","
		}
		queryDCs = queryDCs + fmt.Sprintf("'%s': %d", dc.Name, dc.RF)
	}

	query := fmt.Sprintf("ALTER KEYSPACE system_auth WITH replication = { 'class': '%s' , %s  } ;", ReplicationClassNetworkTopologyStrategy, queryDCs)

	return c.Session.Query(query).Exec()
}

func (c *cassandraClient) GetUsers() ([]CassandraUser, error) {
	iter := c.Session.Query("SELECT role,is_superuser FROM system_auth.roles").Iter()

	cassUsers := make([]CassandraUser, 0, iter.NumRows())
	var role string
	var isSuperuser bool
	for iter.Scan(&role, &isSuperuser) {
		cassUsers = append(cassUsers, CassandraUser{Role: role, IsSuperuser: isSuperuser})
	}

	if err := iter.Close(); err != nil {
		return []CassandraUser{}, errors.Wrapf(err, "Can't close iterator")
	}
	return cassUsers, nil
}
