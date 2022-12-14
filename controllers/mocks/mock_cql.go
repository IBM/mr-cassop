// Code generated by MockGen. DO NOT EDIT.
// Source: ./controllers/cql/cql.go

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	cql "github.com/ibm/cassandra-operator/controllers/cql"
)

// MockCqlClient is a mock of CqlClient interface.
type MockCqlClient struct {
	ctrl     *gomock.Controller
	recorder *MockCqlClientMockRecorder
}

// MockCqlClientMockRecorder is the mock recorder for MockCqlClient.
type MockCqlClientMockRecorder struct {
	mock *MockCqlClient
}

// NewMockCqlClient creates a new mock instance.
func NewMockCqlClient(ctrl *gomock.Controller) *MockCqlClient {
	mock := &MockCqlClient{ctrl: ctrl}
	mock.recorder = &MockCqlClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockCqlClient) EXPECT() *MockCqlClientMockRecorder {
	return m.recorder
}

// CloseSession mocks base method.
func (m *MockCqlClient) CloseSession() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "CloseSession")
}

// CloseSession indicates an expected call of CloseSession.
func (mr *MockCqlClientMockRecorder) CloseSession() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CloseSession", reflect.TypeOf((*MockCqlClient)(nil).CloseSession))
}

// CreateRole mocks base method.
func (m *MockCqlClient) CreateRole(role cql.Role) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateRole", role)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateRole indicates an expected call of CreateRole.
func (mr *MockCqlClientMockRecorder) CreateRole(role interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateRole", reflect.TypeOf((*MockCqlClient)(nil).CreateRole), role)
}

// DropRole mocks base method.
func (m *MockCqlClient) DropRole(role cql.Role) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DropRole", role)
	ret0, _ := ret[0].(error)
	return ret0
}

// DropRole indicates an expected call of DropRole.
func (mr *MockCqlClientMockRecorder) DropRole(role interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DropRole", reflect.TypeOf((*MockCqlClient)(nil).DropRole), role)
}

// GetKeyspacesInfo mocks base method.
func (m *MockCqlClient) GetKeyspacesInfo() ([]cql.Keyspace, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetKeyspacesInfo")
	ret0, _ := ret[0].([]cql.Keyspace)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetKeyspacesInfo indicates an expected call of GetKeyspacesInfo.
func (mr *MockCqlClientMockRecorder) GetKeyspacesInfo() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetKeyspacesInfo", reflect.TypeOf((*MockCqlClient)(nil).GetKeyspacesInfo))
}

// GetRoles mocks base method.
func (m *MockCqlClient) GetRoles() ([]cql.Role, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRoles")
	ret0, _ := ret[0].([]cql.Role)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRoles indicates an expected call of GetRoles.
func (mr *MockCqlClientMockRecorder) GetRoles() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRoles", reflect.TypeOf((*MockCqlClient)(nil).GetRoles))
}

// Query mocks base method.
func (m *MockCqlClient) Query(stmt string, values ...interface{}) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{stmt}
	for _, a := range values {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Query", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Query indicates an expected call of Query.
func (mr *MockCqlClientMockRecorder) Query(stmt interface{}, values ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{stmt}, values...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Query", reflect.TypeOf((*MockCqlClient)(nil).Query), varargs...)
}

// UpdateRF mocks base method.
func (m *MockCqlClient) UpdateRF(keyspaceName string, strategyOptions map[string]string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateRF", keyspaceName, strategyOptions)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateRF indicates an expected call of UpdateRF.
func (mr *MockCqlClientMockRecorder) UpdateRF(keyspaceName, strategyOptions interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateRF", reflect.TypeOf((*MockCqlClient)(nil).UpdateRF), keyspaceName, strategyOptions)
}

// UpdateRole mocks base method.
func (m *MockCqlClient) UpdateRole(role cql.Role) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateRole", role)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateRole indicates an expected call of UpdateRole.
func (mr *MockCqlClientMockRecorder) UpdateRole(role interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateRole", reflect.TypeOf((*MockCqlClient)(nil).UpdateRole), role)
}

// UpdateRolePassword mocks base method.
func (m *MockCqlClient) UpdateRolePassword(roleName, newPassword string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateRolePassword", roleName, newPassword)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateRolePassword indicates an expected call of UpdateRolePassword.
func (mr *MockCqlClientMockRecorder) UpdateRolePassword(roleName, newPassword interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateRolePassword", reflect.TypeOf((*MockCqlClient)(nil).UpdateRolePassword), roleName, newPassword)
}
