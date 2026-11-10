// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package d3d11

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/go-mswin/d3d11/dxgi"
	"github.com/go-mswin/d3d11/internal/com"
)

const (
	uavDimensionTexture2D = 4 // D3D11_UAV_DIMENSION_TEXTURE2D
	bytesPerPixel         = 4 // R8G8B8A8_UNORM
)

// texture2DDesc is D3D11_TEXTURE2D_DESC.
type texture2DDesc struct {
	Width, Height, MipLevels, ArraySize, Format uint32
	SampleCount, SampleQuality                  uint32
	Usage, BindFlags, CPUAccessFlags, MiscFlags uint32
}

// Texture is an ID3D11Texture2D that a swap chain owns the pixels of: its back
// buffer, R8G8B8A8_UNORM. Close it before the swap chain is resized.
type Texture struct {
	dev  *Device
	ptr  unsafe.Pointer
	w, h uint32
}

// Size is the texture's size in pixels.
func (t *Texture) Size() (w, h uint32) { return t.w, t.h }

// Close releases the texture. It is idempotent.
func (t *Texture) Close() {
	if t != nil && t.ptr != nil {
		com.Release(t.ptr)
		t.ptr = nil
	}
}

// UAV creates an unordered access view of the texture, format R8G8B8A8_UNORM,
// for a compute shader's RWTexture2D<unorm float4>. A device that cannot store
// to that format as a typed UAV refuses here (Direct3D 11.0 requires typed
// stores only for R32 formats), and the error says so.
func (t *Texture) UAV() (*UAV, error) {
	if t == nil || t.ptr == nil || t.dev.open() != nil {
		return nil, ErrClosed
	}
	vd := uavDesc{Format: dxgi.FormatR8G8B8A8UNorm, ViewDimension: uavDimensionTexture2D}
	var p unsafe.Pointer
	h := com.Call3(t.dev.ptr, devCreateUnorderedAccessView, uintptr(t.ptr), uintptr(unsafe.Pointer(&vd)), uintptr(unsafe.Pointer(&p)))
	if err := t.dev.fail("CreateUnorderedAccessView", h); err != nil {
		return nil, err
	}
	if p == nil {
		return nil, com.Check("CreateUnorderedAccessView", com.EFail)
	}
	com.Adopt()
	return &UAV{ptr: p}, nil
}

// SwapChain is a flip-model swap chain on a window (milestone D4): FLIP_DISCARD,
// R8G8B8A8_UNORM, back buffers usable as UAVs. See package dxgi for the rules.
type SwapChain struct {
	dev  *Device
	sc   *dxgi.SwapChain
	w, h uint32
}

// CreateSwapChain makes a flip-model swap chain for the window hwnd (an HWND),
// on this device's own factory. One flip swap chain per window.
func (d *Device) CreateSwapChain(hwnd uintptr, desc dxgi.SwapChainDesc) (*SwapChain, error) {
	if err := d.open(); err != nil {
		return nil, err
	}
	f, err := dxgi.FactoryOfDevice(d.ptr)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc, err := f.CreateSwapChainForHwnd(d.ptr, hwnd, desc)
	if err != nil {
		return nil, err
	}
	return &SwapChain{dev: d, sc: sc, w: desc.Width, h: desc.Height}, nil
}

// Size is the back buffers' size in pixels.
func (s *SwapChain) Size() (w, h uint32) { return s.w, s.h }

// BackBuffer returns the buffer to draw into next (buffer 0 of the flip model,
// which the runtime rotates at Present: ask again after every Present). Close
// the Texture, and every view of it, before Resize.
func (s *SwapChain) BackBuffer() (*Texture, error) {
	if s == nil || s.sc == nil {
		return nil, ErrClosed
	}
	p, err := s.sc.BackBuffer(0)
	if err != nil {
		return nil, err
	}
	return &Texture{dev: s.dev, ptr: p, w: s.w, h: s.h}, nil
}

// Present shows the back buffer: syncInterval 0 at once, 1 at the next
// vertical blank. The back buffer's content is undefined afterwards (DISCARD).
func (s *SwapChain) Present(syncInterval uint32) error {
	if s == nil || s.sc == nil {
		return ErrClosed
	}
	return s.sc.Present(syncInterval)
}

// Resize changes the back buffers to w × h (ResizeBuffers). The immediate
// context is cleared first (ID3D11DeviceContext::ClearState), because a bound
// view of a back buffer is a reference that makes ResizeBuffers fail; the
// caller must have closed every Texture and view it holds.
func (s *SwapChain) Resize(w, h uint32) error {
	if s == nil || s.sc == nil {
		return ErrClosed
	}
	if w == 0 || h == 0 || w > dxgi.MaxBackBufferSide || h > dxgi.MaxBackBufferSide {
		return fmt.Errorf("%w: swap chain size %d x %d", ErrBadDesc, w, h)
	}
	if err := s.dev.ctx.ClearState(); err != nil {
		return err
	}
	if err := s.sc.ResizeBuffers(w, h); err != nil {
		return err
	}
	s.w, s.h = w, h
	return nil
}

// Close releases the swap chain. It is idempotent. Flush the device afterwards
// if another flip swap chain is to be made for the same window.
func (s *SwapChain) Close() {
	if s != nil && s.sc != nil {
		s.sc.Close()
		s.sc = nil
	}
}

// ClearState unbinds everything from the immediate context.
func (c *Context) ClearState() error {
	if err := c.open(); err != nil {
		return err
	}
	com.Call0(c.ptr, ctxClearState)
	return nil
}

// ReadbackTexture copies a back buffer into a temporary staging texture, waits
// for the GPU and returns its pixels as tightly packed rows, 4·w·h bytes of
// R,G,B,A top to bottom. Do it before Present: the content of a discarded back
// buffer is undefined after.
func (c *Context) ReadbackTexture(t *Texture, timeout time.Duration) ([]byte, error) {
	if err := c.open(); err != nil {
		return nil, err
	}
	if t == nil || t.ptr == nil {
		return nil, ErrClosed
	}
	d := c.dev
	td := texture2DDesc{Width: t.w, Height: t.h, MipLevels: 1, ArraySize: 1, Format: dxgi.FormatR8G8B8A8UNorm,
		SampleCount: 1, Usage: usageStaging, CPUAccessFlags: cpuAccessRead}
	var st unsafe.Pointer
	if err := d.fail("CreateTexture2D", com.Call3(d.ptr, devCreateTexture2D, uintptr(unsafe.Pointer(&td)), 0, uintptr(unsafe.Pointer(&st)))); err != nil {
		return nil, err
	}
	if st == nil {
		return nil, com.Check("CreateTexture2D", com.EFail)
	}
	com.Adopt()
	defer com.Release(st)
	com.Call2(c.ptr, ctxCopyResource, uintptr(st), uintptr(t.ptr))
	if err := c.Wait(timeout); err != nil {
		return nil, err
	}
	var ms mappedSubresource
	if err := d.fail("Map", com.Call5(c.ptr, ctxMap, uintptr(st), 0, mapRead, 0, uintptr(unsafe.Pointer(&ms)))); err != nil {
		return nil, err
	}
	if ms.Data == nil {
		return nil, com.Check("Map", com.EFail)
	}
	defer com.Call2(c.ptr, ctxUnmap, uintptr(st), 0)
	row := int(t.w) * bytesPerPixel
	out := make([]byte, row*int(t.h))
	for y := 0; y < int(t.h); y++ {
		copy(out[y*row:(y+1)*row], unsafe.Slice((*byte)(unsafe.Add(ms.Data, y*int(ms.RowPitch))), row))
	}
	return out, nil
}
