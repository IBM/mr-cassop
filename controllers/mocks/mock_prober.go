// Code generated by MockGen. DO NOT EDIT.
// Source: ./controllers/prober/prober.go

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
)

// MockProberClient is a mock of ProberClient interface.
type MockProberClient struct {
	ctrl     *gomock.Controller
	recorder *MockProberClientMockRecorder
}

// MockProberClientMockRecorder is the mock recorder for MockProberClient.
type MockProberClientMockRecorder struct {
	mock *MockProberClient
}

// NewMockProberClient creates a new mock instance.
func NewMockProberClient(ctrl *gomock.Controller) *MockProberClient {
	mock := &MockProberClient{ctrl: ctrl}
	mock.recorder = &MockProberClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockProberClient) EXPECT() *MockProberClientMockRecorder {
	return m.recorder
}

// GetDCs mocks base method.
func (m *MockProberClient) GetDCs(ctx context.Context, host string) ([]v1alpha1.DC, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetDCs", ctx, host)
	ret0, _ := ret[0].([]v1alpha1.DC)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetDCs indicates an expected call of GetDCs.
func (mr *MockProberClientMockRecorder) GetDCs(ctx, host interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetDCs", reflect.TypeOf((*MockProberClient)(nil).GetDCs), ctx, host)
}

// GetReaperIPs mocks base method.
func (m *MockProberClient) GetReaperIPs(ctx context.Context, host string) ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetReaperIPs", ctx, host)
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetReaperIPs indicates an expected call of GetReaperIPs.
func (mr *MockProberClientMockRecorder) GetReaperIPs(ctx, host interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetReaperIPs", reflect.TypeOf((*MockProberClient)(nil).GetReaperIPs), ctx, host)
}

// GetRegionIPs mocks base method.
func (m *MockProberClient) GetRegionIPs(ctx context.Context, host string) ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRegionIPs", ctx, host)
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRegionIPs indicates an expected call of GetRegionIPs.
func (mr *MockProberClientMockRecorder) GetRegionIPs(ctx, host interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRegionIPs", reflect.TypeOf((*MockProberClient)(nil).GetRegionIPs), ctx, host)
}

// GetSeeds mocks base method.
func (m *MockProberClient) GetSeeds(ctx context.Context, host string) ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSeeds", ctx, host)
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSeeds indicates an expected call of GetSeeds.
func (mr *MockProberClientMockRecorder) GetSeeds(ctx, host interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSeeds", reflect.TypeOf((*MockProberClient)(nil).GetSeeds), ctx, host)
}

// Ready mocks base method.
func (m *MockProberClient) Ready(ctx context.Context) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Ready", ctx)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Ready indicates an expected call of Ready.
func (mr *MockProberClientMockRecorder) Ready(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Ready", reflect.TypeOf((*MockProberClient)(nil).Ready), ctx)
}

// ReaperReady mocks base method.
func (m *MockProberClient) ReaperReady(ctx context.Context, host string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReaperReady", ctx, host)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReaperReady indicates an expected call of ReaperReady.
func (mr *MockProberClientMockRecorder) ReaperReady(ctx, host interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReaperReady", reflect.TypeOf((*MockProberClient)(nil).ReaperReady), ctx, host)
}

// RegionReady mocks base method.
func (m *MockProberClient) RegionReady(ctx context.Context, host string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegionReady", ctx, host)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RegionReady indicates an expected call of RegionReady.
func (mr *MockProberClientMockRecorder) RegionReady(ctx, host interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegionReady", reflect.TypeOf((*MockProberClient)(nil).RegionReady), ctx, host)
}

// UpdateDCs mocks base method.
func (m *MockProberClient) UpdateDCs(ctx context.Context, dcs []v1alpha1.DC) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateDCs", ctx, dcs)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateDCs indicates an expected call of UpdateDCs.
func (mr *MockProberClientMockRecorder) UpdateDCs(ctx, dcs interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateDCs", reflect.TypeOf((*MockProberClient)(nil).UpdateDCs), ctx, dcs)
}

// UpdateReaperIPs mocks base method.
func (m *MockProberClient) UpdateReaperIPs(ctx context.Context, ips []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateReaperIPs", ctx, ips)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateReaperIPs indicates an expected call of UpdateReaperIPs.
func (mr *MockProberClientMockRecorder) UpdateReaperIPs(ctx, ips interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateReaperIPs", reflect.TypeOf((*MockProberClient)(nil).UpdateReaperIPs), ctx, ips)
}

// UpdateReaperStatus mocks base method.
func (m *MockProberClient) UpdateReaperStatus(ctx context.Context, ready bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateReaperStatus", ctx, ready)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateReaperStatus indicates an expected call of UpdateReaperStatus.
func (mr *MockProberClientMockRecorder) UpdateReaperStatus(ctx, ready interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateReaperStatus", reflect.TypeOf((*MockProberClient)(nil).UpdateReaperStatus), ctx, ready)
}

// UpdateRegionIPs mocks base method.
func (m *MockProberClient) UpdateRegionIPs(ctx context.Context, ips []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateRegionIPs", ctx, ips)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateRegionIPs indicates an expected call of UpdateRegionIPs.
func (mr *MockProberClientMockRecorder) UpdateRegionIPs(ctx, ips interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateRegionIPs", reflect.TypeOf((*MockProberClient)(nil).UpdateRegionIPs), ctx, ips)
}

// UpdateRegionStatus mocks base method.
func (m *MockProberClient) UpdateRegionStatus(ctx context.Context, ready bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateRegionStatus", ctx, ready)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateRegionStatus indicates an expected call of UpdateRegionStatus.
func (mr *MockProberClientMockRecorder) UpdateRegionStatus(ctx, ready interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateRegionStatus", reflect.TypeOf((*MockProberClient)(nil).UpdateRegionStatus), ctx, ready)
}

// UpdateSeeds mocks base method.
func (m *MockProberClient) UpdateSeeds(ctx context.Context, seeds []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateSeeds", ctx, seeds)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateSeeds indicates an expected call of UpdateSeeds.
func (mr *MockProberClientMockRecorder) UpdateSeeds(ctx, seeds interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateSeeds", reflect.TypeOf((*MockProberClient)(nil).UpdateSeeds), ctx, seeds)
}
