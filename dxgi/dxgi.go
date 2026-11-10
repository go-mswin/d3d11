// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package dxgi is a pure-Go (no cgo) binding of the part of DXGI that
// Direct3D 11 compute needs: the factory and adapter enumeration, and (v0.3.0,
// milestone D4) a flip-model swap chain on a window: IDXGIFactory2's
// CreateSwapChainForHwnd with DXGI_SWAP_EFFECT_FLIP_DISCARD, R8G8B8A8_UNORM,
// two buffers usable as render target, shader input and unordered access;
// GetBuffer, Present and ResizeBuffers. The rules it follows, from Microsoft
// Learn ("DXGI flip model", "IDXGISwapChain::ResizeBuffers"): the buffer count
// is at least 2, the format is one of R16G16B16A16_FLOAT, B8G8R8A8_UNORM and
// R8G8B8A8_UNORM, no multisampling, one flip swap chain per window, and
// ResizeBuffers fails while any reference to a back buffer (or a view of it, or
// a binding of it) is alive.
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

// Swap chain constants (D4).
const (
	// FormatR8G8B8A8UNorm is DXGI_FORMAT_R8G8B8A8_UNORM, one of the three formats
	// a flip-model swap chain takes (the others are B8G8R8A8_UNORM and
	// R16G16B16A16_FLOAT); the swap chain here uses it only.
	FormatR8G8B8A8UNorm = 28
	// MaxSwapChainBuffers is DXGI_MAX_SWAP_CHAIN_BUFFERS.
	MaxSwapChainBuffers = 16
	// MaxBackBufferSide is D3D11_REQ_TEXTURE2D_U_OR_V_DIMENSION.
	MaxBackBufferSide = 16384
)

// ErrBadDesc reports a swap chain description that breaks a documented rule.
var ErrBadDesc = errors.New("dxgi: invalid swap chain description")

// SwapChainDesc describes a flip-model swap chain on a window: the back
// buffers are R8G8B8A8_UNORM, single-sampled, used as render target, shader
// input and unordered access, presented with DXGI_SWAP_EFFECT_FLIP_DISCARD.
type SwapChainDesc struct {
	// Width and Height are the back buffers' size in pixels, 1 to
	// MaxBackBufferSide.
	Width, Height uint32
	// BufferCount is 2 to MaxSwapChainBuffers; 0 means 2. A flip-model swap chain
	// cannot have fewer than two.
	BufferCount uint32
}

// validated returns d with the defaults applied, or an error matching
// ErrBadDesc.
func (d SwapChainDesc) validated() (SwapChainDesc, error) {
	if d.Width == 0 || d.Height == 0 || d.Width > MaxBackBufferSide || d.Height > MaxBackBufferSide {
		return d, errors.Join(ErrBadDesc, errors.New("size must be 1 to 16384 in each direction"))
	}
	if d.BufferCount == 0 {
		d.BufferCount = 2
	}
	if d.BufferCount < 2 || d.BufferCount > MaxSwapChainBuffers {
		return d, errors.Join(ErrBadDesc, errors.New("a flip-model swap chain has 2 to 16 buffers"))
	}
	return d, nil
}
