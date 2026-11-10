// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package dxgi

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/go-mswin/d3d11/internal/com"
	"golang.org/x/sys/windows"
)

var (
	modDXGI                = windows.NewLazySystemDLL("dxgi.dll")
	procCreateDXGIFactory1 = modDXGI.NewProc("CreateDXGIFactory1")
)

// Interface identities.
var (
	iidIDXGIFactory1 = com.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
	iidIDXGIDevice   = com.GUID{Data1: 0x54ec77fa, Data2: 0x1377, Data3: 0x44e6, Data4: [8]byte{0x8c, 0x32, 0x88, 0xfd, 0x5f, 0x44, 0xc8, 0x4c}}
	iidIDXGIAdapter1 = com.GUID{Data1: 0x29038f61, Data2: 0x3839, Data3: 0x4626, Data4: [8]byte{0x91, 0xfd, 0x08, 0x68, 0x79, 0x01, 0x1a, 0x05}}
)

// Vtable slots, each with the inheritance chain that produces it.
const (
	// IUnknown: QueryInterface(0) AddRef(1) Release(2)
	// IDXGIObject: SetPrivateData(3) SetPrivateDataInterface(4)
	//              GetPrivateData(5) GetParent(6)
	// IDXGIFactory: EnumAdapters(7) MakeWindowAssociation(8)
	//               GetWindowAssociation(9) CreateSwapChain(10)
	//               CreateSoftwareAdapter(11)
	// IDXGIFactory1: EnumAdapters1(12) IsCurrent(13)
	slotFactory1EnumAdapters1 = 12

	// IUnknown(0..2), IDXGIObject(3..6), IDXGIAdapter: EnumOutputs(7)
	// GetDesc(8) CheckInterfaceSupport(9); IDXGIAdapter1: GetDesc1(10)
	slotAdapter1GetDesc1 = 10

	// IUnknown(0..2), IDXGIObject(3..6), IDXGIDevice: GetAdapter(7)
	// CreateSurface(8) QueryResourceResidency(9) SetGPUThreadPriority(10)
	// GetGPUThreadPriority(11)
	slotDeviceGetAdapter = 7
)

// descriptor1 is DXGI_ADAPTER_DESC1 (312 bytes on 64-bit).
type descriptor1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLuid           [2]uint32
	Flags                 uint32
}

func (d *descriptor1) public() AdapterDesc {
	return AdapterDesc{
		Description:          windows.UTF16ToString(d.Description[:]),
		VendorID:             d.VendorID,
		DeviceID:             d.DeviceID,
		DedicatedVideoMemory: uint64(d.DedicatedVideoMemory),
		Flags:                d.Flags,
	}
}

// Factory is an IDXGIFactory1. The swap chain (D4) hangs off IDXGIFactory2,
// reached by com.QueryInterface on ptr.
type Factory struct{ ptr unsafe.Pointer }

// CreateFactory1 creates the factory.
func CreateFactory1() (*Factory, error) {
	if err := procCreateDXGIFactory1.Find(); err != nil {
		return nil, fmt.Errorf("%w: CreateDXGIFactory1: %v", ErrUnsupported, err)
	}
	var p unsafe.Pointer
	r, _, _ := procCreateDXGIFactory1.Call(uintptr(unsafe.Pointer(&iidIDXGIFactory1)), uintptr(unsafe.Pointer(&p)))
	if err := com.Check("CreateDXGIFactory1", com.HRESULT(int32(uint32(r)))); err != nil {
		return nil, err
	}
	if p == nil {
		return nil, com.Check("CreateDXGIFactory1", com.EFail)
	}
	com.Adopt()
	return &Factory{ptr: p}, nil
}

// Close releases the factory. It is idempotent.
func (f *Factory) Close() {
	if f != nil && f.ptr != nil {
		com.Release(f.ptr)
		f.ptr = nil
	}
}

// Adapter is an owned IDXGIAdapter1.
type Adapter struct{ ptr unsafe.Pointer }

// EnumAdapter returns adapter i, or an error matching [ErrNotFound] past the
// end.
func (f *Factory) EnumAdapter(i int) (*Adapter, error) {
	if f == nil || f.ptr == nil {
		return nil, com.Check("EnumAdapters1", com.EInvalidArg)
	}
	var p unsafe.Pointer
	h := com.Call2(f.ptr, slotFactory1EnumAdapters1, uintptr(uint32(i)), uintptr(unsafe.Pointer(&p)))
	if err := com.Check("EnumAdapters1", h); err != nil {
		return nil, err
	}
	if p == nil {
		return nil, com.Check("EnumAdapters1", com.EFail)
	}
	com.Adopt()
	return &Adapter{ptr: p}, nil
}

// Adapters describes every adapter, at most [MaxAdapters].
func (f *Factory) Adapters() ([]AdapterDesc, error) {
	var out []AdapterDesc
	for i := 0; i < MaxAdapters; i++ {
		a, err := f.EnumAdapter(i)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return out, nil
			}
			return out, err
		}
		d, err := a.Desc()
		a.Close()
		if err != nil {
			return out, err
		}
		out = append(out, d)
	}
	return out, nil
}

// Desc is IDXGIAdapter1::GetDesc1.
func (a *Adapter) Desc() (AdapterDesc, error) { return describe(a.ptr) }

// Ptr is the IDXGIAdapter1 pointer, for D3D11CreateDevice. It stays owned by
// the Adapter.
func (a *Adapter) Ptr() unsafe.Pointer { return a.ptr }

// Close releases the adapter. It is idempotent.
func (a *Adapter) Close() {
	if a != nil && a.ptr != nil {
		com.Release(a.ptr)
		a.ptr = nil
	}
}

func describe(adapter1 unsafe.Pointer) (AdapterDesc, error) {
	var d descriptor1
	h := com.Call1(adapter1, slotAdapter1GetDesc1, uintptr(unsafe.Pointer(&d)))
	if err := com.Check("IDXGIAdapter1::GetDesc1", h); err != nil {
		return AdapterDesc{}, err
	}
	return d.public(), nil
}

// DescribeDeviceAdapter describes the adapter a Direct3D device runs on. device
// is an ID3D11Device (or any IDXGIDevice) pointer; nothing is retained and
// every temporary interface is released.
func DescribeDeviceAdapter(device unsafe.Pointer) (AdapterDesc, error) {
	dxgiDev, err := com.QueryInterface(device, &iidIDXGIDevice, "QueryInterface(IDXGIDevice)")
	if err != nil {
		return AdapterDesc{}, err
	}
	defer com.Release(dxgiDev)
	var ad unsafe.Pointer
	if err := com.Check("IDXGIDevice::GetAdapter", com.Call1(dxgiDev, slotDeviceGetAdapter, uintptr(unsafe.Pointer(&ad)))); err != nil {
		return AdapterDesc{}, err
	}
	if ad == nil {
		return AdapterDesc{}, com.Check("IDXGIDevice::GetAdapter", com.EFail)
	}
	com.Adopt()
	defer com.Release(ad)
	ad1, err := com.QueryInterface(ad, &iidIDXGIAdapter1, "QueryInterface(IDXGIAdapter1)")
	if err != nil {
		return AdapterDesc{}, err
	}
	defer com.Release(ad1)
	return describe(ad1)
}
