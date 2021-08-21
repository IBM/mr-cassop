package cql

import (
	"fmt"
	"github.com/gocql/gocql"
	"github.com/pkg/errors"
)

const (
	ReplicationClassNetworkTopologyStrategy = "org.apache.cassandra.locator.NetworkTopologyStrategy"
)

type CqlClient interface {
	GetKeyspacesInfo() ([]Keyspace, error)
	UpdateRF(keyspaceName string, strategyOptions map[string]string) error
	GetRoles() ([]Role, error)
	CreateRole(role Role) error
	UpdateRole(role Role) error
	UpdateRolePassword(roleName, newPassword string) error
	Query(stmt string, values ...interface{}) error
	DropRole(role Role) error
	CloseSession()
}

type cassandraClient struct {
	*gocql.Session
}

type Role struct {
	Role  string
	Super bool
	Login bool
	// empty when retrieving roles. Only used when create/update a role.
	Password string
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

func (c cassandraClient) UpdateRF(keyspaceName string, rfOptions map[string]string) error {
	rfOptionsQuery := ""
	for optionKey, optionValue := range rfOptions {
		if rfOptionsQuery != "" {
			rfOptionsQuery = rfOptionsQuery + ","
		}
		rfOptionsQuery = rfOptionsQuery + fmt.Sprintf("'%s' : '%s'", optionKey, optionValue)
	}

	query := fmt.Sprintf("ALTER KEYSPACE %s WITH replication = { %s } ;", keyspaceName, rfOptionsQuery)
	return c.Session.Query(query).Exec()
}

func (c *cassandraClient) GetRoles() ([]Role, error) {
	iter := c.Session.Query("LIST ROLES").Iter()

	cassandraRoles := make([]Role, 0, iter.NumRows())
	var role string
	var isSuperuser bool
	var login bool
	var options map[string]string
	for iter.Scan(&role, &isSuperuser, &login, &options) {
		cassandraRoles = append(cassandraRoles, Role{Role: role, Super: isSuperuser})
	}

	if err := iter.Close(); err != nil {
		return []Role{}, errors.Wrapf(err, "Can't close iterator")
	}
	return cassandraRoles, nil
}

func (c *cassandraClient) CreateRole(role Role) error {
	query := fmt.Sprintf("CREATE ROLE '%s' WITH PASSWORD = '%s' AND LOGIN = %t AND SUPERUSER= %t", role.Role, role.Password, role.Login, role.Super)
	return c.Session.Query(query).Exec()
}

func (c *cassandraClient) UpdateRole(role Role) error {
	passwordQuery := ""
	if role.Password != "" {
		passwordQuery = fmt.Sprintf("AND PASSWORD = '%s'", role.Password)
	}

	query := fmt.Sprintf("ALTER ROLE '%s' WITH SUPERUSER = %t AND LOGIN = %t %s", role.Role, role.Super, role.Login, passwordQuery)
	return c.Session.Query(query).Exec()
}

func (c *cassandraClient) UpdateRolePassword(roleName, newPassword string) error {
	query := fmt.Sprintf("ALTER ROLE '%s' WITH PASSWORD = '%s'", roleName, newPassword)
	return c.Session.Query(query).Exec()
}

func (c *cassandraClient) DropRole(role Role) error {
	query := fmt.Sprintf("DROP ROLE IF EXISTS %s", role.Role)
	return c.Session.Query(query).Exec()
}

func (c *cassandraClient) CloseSession() {
	c.Session.Close()
}
