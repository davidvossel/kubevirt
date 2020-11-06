// Automatically generated by MockGen. DO NOT EDIT!
// Source: mount.go

package hotplug_volume

import (
	gomock "github.com/golang/mock/gomock"
	types "k8s.io/apimachinery/pkg/types"

	v1 "kubevirt.io/client-go/api/v1"
)

// Mock of VolumeMounter interface
type MockVolumeMounter struct {
	ctrl     *gomock.Controller
	recorder *_MockVolumeMounterRecorder
}

// Recorder for MockVolumeMounter (not exported)
type _MockVolumeMounterRecorder struct {
	mock *MockVolumeMounter
}

func NewMockVolumeMounter(ctrl *gomock.Controller) *MockVolumeMounter {
	mock := &MockVolumeMounter{ctrl: ctrl}
	mock.recorder = &_MockVolumeMounterRecorder{mock}
	return mock
}

func (_m *MockVolumeMounter) EXPECT() *_MockVolumeMounterRecorder {
	return _m.recorder
}

func (_m *MockVolumeMounter) Mount(vmi *v1.VirtualMachineInstance) error {
	ret := _m.ctrl.Call(_m, "Mount", vmi)
	ret0, _ := ret[0].(error)
	return ret0
}

func (_mr *_MockVolumeMounterRecorder) Mount(arg0 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "Mount", arg0)
}

func (_m *MockVolumeMounter) Unmount(vmi *v1.VirtualMachineInstance) error {
	ret := _m.ctrl.Call(_m, "Unmount", vmi)
	ret0, _ := ret[0].(error)
	return ret0
}

func (_mr *_MockVolumeMounterRecorder) Unmount(arg0 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "Unmount", arg0)
}

func (_m *MockVolumeMounter) UnmountAll(vmi *v1.VirtualMachineInstance) error {
	ret := _m.ctrl.Call(_m, "UnmountAll", vmi)
	ret0, _ := ret[0].(error)
	return ret0
}

func (_mr *_MockVolumeMounterRecorder) UnmountAll(arg0 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "UnmountAll", arg0)
}

func (_m *MockVolumeMounter) IsMounted(vmi *v1.VirtualMachineInstance, volume string, sourceUID types.UID) (bool, error) {
	ret := _m.ctrl.Call(_m, "IsMounted", vmi, volume, sourceUID)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockVolumeMounterRecorder) IsMounted(arg0, arg1, arg2 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "IsMounted", arg0, arg1, arg2)
}
