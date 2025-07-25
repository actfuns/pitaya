// Code generated by MockGen. DO NOT EDIT.
// Source: cluster/service_discovery.go

// Package mock_cluster is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	cluster "github.com/topfreegames/pitaya/v2/cluster"
)

// MockServiceDiscovery is a mock of ServiceDiscovery interface.
type MockServiceDiscovery struct {
	ctrl     *gomock.Controller
	recorder *MockServiceDiscoveryMockRecorder
}

// MockServiceDiscoveryMockRecorder is the mock recorder for MockServiceDiscovery.
type MockServiceDiscoveryMockRecorder struct {
	mock *MockServiceDiscovery
}

// NewMockServiceDiscovery creates a new mock instance.
func NewMockServiceDiscovery(ctrl *gomock.Controller) *MockServiceDiscovery {
	mock := &MockServiceDiscovery{ctrl: ctrl}
	mock.recorder = &MockServiceDiscoveryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockServiceDiscovery) EXPECT() *MockServiceDiscoveryMockRecorder {
	return m.recorder
}

// AddListener mocks base method.
func (m *MockServiceDiscovery) AddListener(listener cluster.SDListener) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AddListener", listener)
}

// AddListener indicates an expected call of AddListener.
func (mr *MockServiceDiscoveryMockRecorder) AddListener(listener interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddListener", reflect.TypeOf((*MockServiceDiscovery)(nil).AddListener), listener)
}

// AfterInit mocks base method.
func (m *MockServiceDiscovery) AfterInit() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AfterInit")
}

// AfterInit indicates an expected call of AfterInit.
func (mr *MockServiceDiscoveryMockRecorder) AfterInit() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AfterInit", reflect.TypeOf((*MockServiceDiscovery)(nil).AfterInit))
}

// BeforeShutdown mocks base method.
func (m *MockServiceDiscovery) BeforeShutdown() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "BeforeShutdown")
}

// BeforeShutdown indicates an expected call of BeforeShutdown.
func (mr *MockServiceDiscoveryMockRecorder) BeforeShutdown() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BeforeShutdown", reflect.TypeOf((*MockServiceDiscovery)(nil).BeforeShutdown))
}

// GetServer mocks base method.
func (m *MockServiceDiscovery) GetServer(id string) (*cluster.Server, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetServer", id)
	ret0, _ := ret[0].(*cluster.Server)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetServer indicates an expected call of GetServer.
func (mr *MockServiceDiscoveryMockRecorder) GetServer(id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetServer", reflect.TypeOf((*MockServiceDiscovery)(nil).GetServer), id)
}

// GetServers mocks base method.
func (m *MockServiceDiscovery) GetServers() []*cluster.Server {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetServers")
	ret0, _ := ret[0].([]*cluster.Server)
	return ret0
}

// GetServers indicates an expected call of GetServers.
func (mr *MockServiceDiscoveryMockRecorder) GetServers() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetServers", reflect.TypeOf((*MockServiceDiscovery)(nil).GetServers))
}

// GetServersByType mocks base method.
func (m *MockServiceDiscovery) GetServersByType(serverType string) (map[string]*cluster.Server, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetServersByType", serverType)
	ret0, _ := ret[0].(map[string]*cluster.Server)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetServersByType indicates an expected call of GetServersByType.
func (mr *MockServiceDiscoveryMockRecorder) GetServersByType(serverType interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetServersByType", reflect.TypeOf((*MockServiceDiscovery)(nil).GetServersByType), serverType)
}

// Init mocks base method.
func (m *MockServiceDiscovery) Init() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Init")
	ret0, _ := ret[0].(error)
	return ret0
}

// Init indicates an expected call of Init.
func (mr *MockServiceDiscoveryMockRecorder) Init() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Init", reflect.TypeOf((*MockServiceDiscovery)(nil).Init))
}

// Shutdown mocks base method.
func (m *MockServiceDiscovery) Shutdown() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Shutdown")
	ret0, _ := ret[0].(error)
	return ret0
}

// Shutdown indicates an expected call of Shutdown.
func (mr *MockServiceDiscoveryMockRecorder) Shutdown() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Shutdown", reflect.TypeOf((*MockServiceDiscovery)(nil).Shutdown))
}

// SyncServers mocks base method.
func (m *MockServiceDiscovery) SyncServers(firstSync bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SyncServers", firstSync)
	ret0, _ := ret[0].(error)
	return ret0
}

// SyncServers indicates an expected call of SyncServers.
func (mr *MockServiceDiscoveryMockRecorder) SyncServers(firstSync interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SyncServers", reflect.TypeOf((*MockServiceDiscovery)(nil).SyncServers), firstSync)
}

// UpdateMetadata mocks base method.
func (m *MockServiceDiscovery) UpdateMetadata(metadata map[string]string) error{
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateMetadata", metadata)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateMetadata indicates an expected call of UpdateMetadata.
func (mr *MockServiceDiscoveryMockRecorder) UpdateMetadata(metadata map[string]string) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateMetadata", reflect.TypeOf((*MockServiceDiscovery)(nil).UpdateMetadata), metadata)
}
