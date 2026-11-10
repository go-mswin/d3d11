// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package dxgi is a pure-Go (no cgo) binding of the part of DXGI that
// Direct3D 11 compute needs: the factory and adapter enumeration. It is the
// home of the swap chain (IDXGIFactory2::CreateSwapChainForHwnd, GetBuffer,
// Present1, ResizeBuffers), which is milestone D4 and not yet bound: add it to
// Factory in dxgi_windows.go, with each vtable slot spelled out with its
// inheritance chain.
//
// Everything is reached through syscall on COM vtable slots, as in
// go-mswin/screencapture. The package builds on every OS; off Windows every
// constructor returns [ErrUnsupportedPlatform].
package dxgi

import (
	"errors"

	"github.com/go-mswin/d3d11/internal/com"
)

// HRESULT, Error and the sentinels are shared with package d3d11.
type (
	HRESULT = com.HRESULT
	Error   = com.Error
)

// Sentinels, test with errors.Is.
var (
	ErrDeviceRemoved = com.ErrDeviceRemoved
	ErrNotFound      = com.ErrNotFound
	ErrUnsupported   = com.ErrUnsupported
	// ErrUnsupportedPlatform is returned by every constructor off Windows.
	ErrUnsupportedPlatform = errors.New("dxgi: only available on Windows")
)

// AdapterFlagSoftware is DXGI_ADAPTER_FLAG_SOFTWARE.
const AdapterFlagSoftware = 2

// AdapterDesc is the useful part of DXGI_ADAPTER_DESC1.
type AdapterDesc struct {
	Description          string
	VendorID, DeviceID   uint32
	DedicatedVideoMemory uint64
	Flags                uint32
}

// Software reports a software adapter (the Microsoft Basic Render Driver).
func (d AdapterDesc) Software() bool { return d.Flags&AdapterFlagSoftware != 0 }

// MaxAdapters bounds an enumeration so a misbehaving factory cannot loop
// forever; real machines have a handful.
const MaxAdapters = 64
