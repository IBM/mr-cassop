package integration

import (
	"context"
	"reflect"
	"strconv"

	"github.com/ibm/cassandra-operator/controllers/nodectl"

	"github.com/gogo/protobuf/proto"

	"github.com/ibm/cassandra-operator/controllers/reaper"

	"github.com/gocql/gocql"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
)

type proberMock struct {
	ready         bool
	seeds         map[string][]string
	dcs           map[string][]dbv1alpha1.DC
	readyClusters map[string]bool
	readyReaper   map[string]bool
	err           error
	regionIPs     map[string][]string
	reaperIPs     map[string][]string
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
	repairSchedules []reaper.RepairSchedule
	isRunning       bool
	clusters        []string
	clusterName     string
	err             error
}

func (r proberMock) Ready(ctx context.Context) (bool, error) {
	return r.ready, r.err
}

func (r proberMock) GetSeeds(ctx context.Context, host string) ([]string, error) {
	return r.seeds[host], r.err
}

func (r proberMock) UpdateSeeds(ctx context.Context, seeds []string) error {
	return r.err
}

func (r proberMock) GetDCs(ctx context.Context, host string) ([]dbv1alpha1.DC, error) {
	return r.dcs[host], r.err
}

func (r proberMock) UpdateDCs(ctx context.Context, dcs []dbv1alpha1.DC) error {
	return r.err
}

func (r proberMock) UpdateRegionStatus(ctx context.Context, ready bool) error {
	return r.err
}

func (r proberMock) RegionReady(ctx context.Context, host string) (bool, error) {
	dcsReady, found := r.readyClusters[host]
	if !found {
		return false, errors.Errorf("Host %q not found", host)
	}
	return dcsReady, nil
}

func (r proberMock) UpdateReaperStatus(ctx context.Context, ready bool) error {
	return r.err
}

func (r proberMock) ReaperReady(ctx context.Context, host string) (bool, error) {
	reaperReady, found := r.readyReaper[host]
	if !found {
		return false, errors.Errorf("Host %q not found", host)
	}
	return reaperReady, nil
}

func (r proberMock) GetRegionIPs(ctx context.Context, host string) ([]string, error) {
	return r.regionIPs[host], r.err
}

func (r proberMock) UpdateRegionIPs(ctx context.Context, ips []string) error {
	return r.err
}

func (r proberMock) GetReaperIPs(ctx context.Context, host string) ([]string, error) {
	return r.reaperIPs[host], r.err
}

func (r proberMock) UpdateReaperIPs(ctx context.Context, ips []string) error {
	return r.err
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

func (c *cqlMock) UpdateRolePassword(roleName, newPassword string) error {
	for i, cassandraRole := range c.cassandraRoles {
		if cassandraRole.Role == roleName {
			c.cassandraRoles[i].Password = newPassword
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

func (c *cqlMock) DropRole(role cql.Role) error {
	for i, cassandraRole := range c.cassandraRoles {
		if cassandraRole.Role == role.Role {
			c.cassandraRoles[i] = cql.Role{}
			return nil
		}
	}

	return gocql.ErrNotFound
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

func (c *cqlMock) CloseSession() {}

func (n *nodetoolMock) RepairKeyspace(cc *dbv1alpha1.CassandraCluster, keyspace string) error {
	return n.err
}

func (r *reaperMock) IsRunning(ctx context.Context) (bool, error) {
	return r.isRunning, r.err
}
func (r *reaperMock) ClusterExists(ctx context.Context) (bool, error) {
	for _, addedCluster := range r.clusters {
		if addedCluster == r.clusterName {
			return true, nil
		}
	}
	return false, r.err
}
func (r *reaperMock) AddCluster(ctx context.Context, seed string) error {
	if !util.Contains(r.clusters, r.clusterName) {
		r.clusters = append(r.clusters, r.clusterName)
	}
	return r.err
}
func (r *reaperMock) Clusters(ctx context.Context) ([]string, error) {
	return r.clusters, r.err
}
func (r *reaperMock) DeleteCluster(ctx context.Context) error {
	r.clusters = []string{}
	return r.err
}
func (r *reaperMock) CreateRepairSchedule(ctx context.Context, repair dbv1alpha1.RepairSchedule) error {
	for _, existingRepair := range r.repairSchedules {
		if existingRepair.KeyspaceName == repair.Keyspace && reflect.DeepEqual(existingRepair.Tables, repair.Tables) {
			return errors.Errorf("Repair schedule for keyspace %s with tables %v already exists", existingRepair.KeyspaceName, repair.Tables)
		}
	}
	var intensity float64
	var err error
	intensity, err = strconv.ParseFloat(repair.Intensity, 64)
	if err != nil {
		intensity = 1.0
	}

	schedule := reaper.RepairSchedule{
		ID:                  "id-" + strconv.Itoa(len(r.repairSchedules)),
		KeyspaceName:        repair.Keyspace,
		SegmentCount:        repair.SegmentCountPerNode,
		Owner:               reaper.OwnerCassandraOperator,
		State:               "ACTIVE",
		Tables:              repair.Tables,
		ScheduleDaysBetween: repair.ScheduleDaysBetween,
		Datacenters:         repair.Datacenters,
		IncrementalRepair:   repair.IncrementalRepair,
		RepairThreadCount:   repair.RepairThreadCount,
		Intensity:           intensity,
		RepairParallelism:   repair.RepairParallelism,
	}

	r.repairSchedules = append(r.repairSchedules, schedule)
	return r.err
}

func (r *reaperMock) RepairSchedules(ctx context.Context) ([]reaper.RepairSchedule, error) {
	return r.repairSchedules, r.err
}

func (r *reaperMock) DeleteRepairSchedule(ctx context.Context, repairScheduleID string) error {
	for i, schedule := range r.repairSchedules {
		if schedule.ID == repairScheduleID {
			r.repairSchedules = append(r.repairSchedules[:i], r.repairSchedules[i+1:]...)
			return nil
		}
	}
	return errors.New("unable to remove repair schedule: not found")
}

func (r *reaperMock) SetRepairScheduleState(ctx context.Context, repairScheduleID string, active bool) error {
	for i, schedule := range r.repairSchedules {
		if schedule.ID == repairScheduleID {
			state := "ACTIVE"
			if !active {
				state = "PAUSED"
			}
			r.repairSchedules[i].State = state
			return nil
		}
	}

	return errors.New("unable to update repair schedule state: not found")
}

func (r *reaperMock) RunRepair(ctx context.Context, keyspace, cause string) error {
	return r.err
}

type mockNode struct {
	clusterView nodectl.ClusterView
	opMode      nodectl.OperationMode
}

type nodectlMock struct {
	nodesState map[string]mockNode
}

func (n *nodectlMock) Decommission(ctx context.Context, nodeIP string) error {
	return nil
}

func (n *nodectlMock) Assassinate(ctx context.Context, execNodeIP string, assassinateNodeIP string) error {
	return nil
}

func (n *nodectlMock) Version(ctx context.Context, nodeIP string) (major, minor, patch int, err error) {
	return 3, 11, 11, nil
}

func (n *nodectlMock) ClusterView(ctx context.Context, nodeIP string) (nodectl.ClusterView, error) {
	return n.nodesState[nodeIP].clusterView, nil
}

func (n *nodectlMock) OperationMode(ctx context.Context, nodeIP string) (nodectl.OperationMode, error) {
	return n.nodesState[nodeIP].opMode, nil
}

func markMocksAsReady(cc *dbv1alpha1.CassandraCluster) {
	for i, externalRegion := range cc.Spec.ExternalRegions.Managed {
		mockProberClient.readyClusters[externalRegion.Domain] = true
		mockProberClient.seeds[externalRegion.Domain] = []string{"13.43.13" + strconv.Itoa(i) + ".3", "13.43.13" + strconv.Itoa(i) + ".4"}
		mockProberClient.dcs[externalRegion.Domain] = []dbv1alpha1.DC{
			{
				Name:     "ext-dc-" + "-" + strconv.Itoa(i),
				Replicas: proto.Int32(3),
			},
		}
	}
	mockProberClient.err = nil
	mockProberClient.ready = true
	mockNodetoolClient.err = nil
	mockReaperClient.err = nil
	mockReaperClient.isRunning = true
	mockReaperClient.clusters = []string{cc.Name}
	mockReaperClient.clusterName = cc.Name
	mockCQLClient.err = nil
	mockCQLClient.cassandraRoles = []cql.Role{{Role: "cassandra", Password: "cassandra", Login: true, Super: true}}
	mockCQLClient.keyspaces = []cql.Keyspace{{
		Name: "system_auth",
		Replication: map[string]string{
			"class": cql.ReplicationClassSimpleTopologyStrategy,
		},
	}}
}
