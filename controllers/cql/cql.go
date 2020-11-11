package cql

import (
	"fmt"
	"github.com/gocql/gocql"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	"time"
)

const (
	ReplicationClassNetworkTopologyStrategy = "org.apache.cassandra.locator.NetworkTopologyStrategy"
)

type CQLClient struct {
	*gocql.Session
}

type CassandraUser struct {
	Role        string
	IsSuperuser bool
}

func NewCQLClient(cluster *v1alpha1.CassandraCluster) (*CQLClient, error) {
	cassCfg := gocql.NewCluster(names.DCService(cluster, cluster.Spec.DCs[0].Name))
	cassCfg.Authenticator = &gocql.PasswordAuthenticator{
		Username: cluster.Spec.Cassandra.Auth.User,
		Password: cluster.Spec.Cassandra.Auth.Password,
	}

	cassCfg.Timeout = 6 * time.Second
	cassCfg.ConnectTimeout = 6 * time.Second
	cassCfg.ProtoVersion = 4
	cassCfg.Consistency = gocql.LocalQuorum

	cassSession, err := cassCfg.CreateSession()
	if err != nil {
		return nil, err
	}

	return &CQLClient{Session: cassSession}, nil
}

type Keyspace struct {
	Name        string
	Replication map[string]string
}

func (c CQLClient) GetKeyspacesInfo() ([]Keyspace, error) {
	iter := c.Query("SELECT keyspace_name,replication FROM system_schema.keyspaces").Iter()

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

func (c CQLClient) UpdateRF(cc *v1alpha1.CassandraCluster) error {
	queryDCs := ""
	for _, dc := range cc.Spec.SystemKeyspaces.DCs {
		if queryDCs != "" {
			queryDCs = queryDCs + ","
		}
		queryDCs = queryDCs + fmt.Sprintf("'%s': %d", dc.Name, dc.RF)
	}

	query := fmt.Sprintf("ALTER KEYSPACE system_auth WITH replication = { 'class': '%s' , %s  } ;", ReplicationClassNetworkTopologyStrategy, queryDCs)

	return c.Query(query).Exec()
}

func (c *CQLClient) GetUsers() ([]CassandraUser, error) {
	iter := c.Query("SELECT role,is_superuser FROM system_auth.roles").Iter()

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
