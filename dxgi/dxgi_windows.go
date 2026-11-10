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

// Swap chain interface identities and vtable slots.
var (
	iidIDXGIFactory2   = com.GUID{Data1: 0x50c83a1c, Data2: 0xe072, Data3: 0x4c48, Data4: [8]byte{0x87, 0xb0, 0x36, 0x30, 0xfa, 0x36, 0xa6, 0xd0}}
	iidID3D11Texture2D = com.GUID{Data1: 0x6f15aaf2, Data2: 0xd208, Data3: 0x4e89, Data4: [8]byte{0x9a, 0xb4, 0x48, 0x95, 0x35, 0xd3, 0x4f, 0x9c}}
)

const (
	// IUnknown(0..2), IDXGIObject: SetPrivateData(3) SetPrivateDataInterface(4)
	// GetPrivateData(5) GetParent(6).
	slotObjectGetParent = 6

	// IDXGIFactory1 ends at IsCurrent(13); IDXGIFactory2: IsWindowedStereoEnabled(14)
	// CreateSwapChainForHwnd(15) CreateSwapChainForCoreWindow(16) ...
	slotFactory2CreateSwapChainForHwnd = 15

	// IUnknown(0..2), IDXGIObject(3..6), IDXGIDeviceSubObject: GetDevice(7),
	// IDXGISwapChain: Present(8) GetBuffer(9) SetFullscreenState(10)
	// GetFullscreenState(11) GetDesc(12) ResizeBuffers(13) ResizeTarget(14)
	// GetContainingOutput(15) GetFrameStatistics(16) GetLastPresentCount(17).
	slotSwapChainPresent       = 8
	slotSwapChainGetBuffer     = 9
	slotSwapChainResizeBuffers = 13
)

// DXGI_USAGE and DXGI_SWAP_EFFECT values (dxgi.h, dxgitype.h).
const (
	usageShaderInput        = 1 << (0 + 4)
	usageRenderTargetOutput = 1 << (1 + 4)
	usageUnorderedAccess    = 1 << (6 + 4)
	swapEffectFlipDiscard   = 4
)

// swapChainDesc1 is DXGI_SWAP_CHAIN_DESC1 (48 bytes).
type swapChainDesc1 struct {
	Width, Height uint32
	Format        uint32
	Stereo        uint32 // BOOL
	SampleCount   uint32
	SampleQuality uint32
	BufferUsage   uint32
	BufferCount   uint32
	Scaling       uint32 // DXGI_SCALING_STRETCH = 0
	SwapEffect    uint32
	AlphaMode     uint32 // DXGI_ALPHA_MODE_UNSPECIFIED = 0
	Flags         uint32
}

// FactoryOfDevice returns the factory that made the adapter a Direct3D device
// runs on (IDXGIDevice::GetAdapter, IDXGIObject::GetParent), which is the one a
// swap chain for that device is made by. device is an ID3D11Device pointer;
// nothing of it is retained.
func FactoryOfDevice(device unsafe.Pointer) (*Factory, error) {
	dxgiDev, err := com.QueryInterface(device, &iidIDXGIDevice, "QueryInterface(IDXGIDevice)")
	if err != nil {
		return nil, err
	}
	defer com.Release(dxgiDev)
	var ad unsafe.Pointer
	if err := com.Check("IDXGIDevice::GetAdapter", com.Call1(dxgiDev, slotDeviceGetAdapter, uintptr(unsafe.Pointer(&ad)))); err != nil {
		return nil, err
	}
	if ad == nil {
		return nil, com.Check("IDXGIDevice::GetAdapter", com.EFail)
	}
	com.Adopt()
	defer com.Release(ad)
	var f unsafe.Pointer
	if err := com.Check("IDXGIAdapter::GetParent", com.Call2(ad, slotObjectGetParent, uintptr(unsafe.Pointer(&iidIDXGIFactory1)), uintptr(unsafe.Pointer(&f)))); err != nil {
		return nil, err
	}
	if f == nil {
		return nil, com.Check("IDXGIAdapter::GetParent", com.EFail)
	}
	com.Adopt()
	return &Factory{ptr: f}, nil
}

// SwapChain is an owned IDXGISwapChain1.
type SwapChain struct{ ptr unsafe.Pointer }

// CreateSwapChainForHwnd creates a flip-model swap chain (FLIP_DISCARD,
// R8G8B8A8_UNORM, d.BufferCount buffers, no multisampling) for the window hwnd
// and the Direct3D 11 device. The back buffers can be render targets, shader
// inputs and UAVs (DXGI_USAGE_UNORDERED_ACCESS): whether a driver honours the
// last for a flip swap chain is not stated by the documentation, so a refusal
// is returned as the error it is. One flip swap chain per window: close the
// previous one (and Flush the device) before making another for the same hwnd.
func (f *Factory) CreateSwapChainForHwnd(device unsafe.Pointer, hwnd uintptr, d SwapChainDesc) (*SwapChain, error) {
	d, err := d.validated()
	if err != nil {
		return nil, err
	}
	if f == nil || f.ptr == nil || device == nil || hwnd == 0 {
		return nil, com.Check("CreateSwapChainForHwnd", com.EInvalidArg)
	}
	f2, err := com.QueryInterface(f.ptr, &iidIDXGIFactory2, "QueryInterface(IDXGIFactory2)")
	if err != nil {
		return nil, err
	}
	defer com.Release(f2)
	desc := swapChainDesc1{
		Width: d.Width, Height: d.Height, Format: FormatR8G8B8A8UNorm, SampleCount: 1,
		BufferUsage: usageShaderInput | usageRenderTargetOutput | usageUnorderedAccess,
		BufferCount: d.BufferCount, SwapEffect: swapEffectFlipDiscard,
	}
	var sc unsafe.Pointer
	h := com.Call6(f2, slotFactory2CreateSwapChainForHwnd, uintptr(device), hwnd, uintptr(unsafe.Pointer(&desc)), 0, 0, uintptr(unsafe.Pointer(&sc)))
	if err := com.Check("CreateSwapChainForHwnd", h); err != nil {
		return nil, err
	}
	if sc == nil {
		return nil, com.Check("CreateSwapChainForHwnd", com.EFail)
	}
	com.Adopt()
	return &SwapChain{ptr: sc}, nil
}

// Present is IDXGISwapChain::Present with no flags: syncInterval 0 presents at
// once, 1 waits for the next vertical blank. The status DXGI_STATUS_OCCLUDED (a
// hidden or covered window) is a success.
func (s *SwapChain) Present(syncInterval uint32) error {
	if s == nil || s.ptr == nil {
		return com.Check("Present", com.EInvalidArg)
	}
	return com.Check("Present", com.Call2(s.ptr, slotSwapChainPresent, uintptr(syncInterval), 0))
}

// BackBuffer is IDXGISwapChain::GetBuffer: an owned reference to the
// ID3D11Texture2D that is buffer index (0 is always the buffer to draw into next
// in the flip model). The caller releases it; every reference must be released,
// with its views, before ResizeBuffers.
func (s *SwapChain) BackBuffer(index uint32) (unsafe.Pointer, error) {
	if s == nil || s.ptr == nil {
		return nil, com.Check("GetBuffer", com.EInvalidArg)
	}
	var t unsafe.Pointer
	if err := com.Check("GetBuffer", com.Call3(s.ptr, slotSwapChainGetBuffer, uintptr(index), uintptr(unsafe.Pointer(&iidID3D11Texture2D)), uintptr(unsafe.Pointer(&t)))); err != nil {
		return nil, err
	}
	if t == nil {
		return nil, com.Check("GetBuffer", com.EFail)
	}
	com.Adopt()
	return t, nil
}

// ResizeBuffers is IDXGISwapChain::ResizeBuffers keeping the buffer count and
// the format. It fails with DXGI_ERROR_INVALID_CALL while a reference to a back
// buffer is alive.
func (s *SwapChain) ResizeBuffers(width, height uint32) error {
	if s == nil || s.ptr == nil {
		return com.Check("ResizeBuffers", com.EInvalidArg)
	}
	return com.Check("ResizeBuffers", com.Call5(s.ptr, slotSwapChainResizeBuffers, 0, uintptr(width), uintptr(height), 0, 0))
}

// Close releases the swap chain. It is idempotent.
func (s *SwapChain) Close() {
	if s != nil && s.ptr != nil {
		com.Release(s.ptr)
		s.ptr = nil
	}
}
