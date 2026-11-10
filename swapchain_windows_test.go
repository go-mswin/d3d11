// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package d3d11

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"unsafe"

	"github.com/go-mswin/d3d11/d3dcompiler"
	"github.com/go-mswin/d3d11/dxgi"
	"golang.org/x/sys/windows"
)

// texKernel writes a pattern of the pixel's position and a seed into a back
// buffer through a typed UAV. Compiled by D3DCompile here, in the test only.
const texKernel = `
RWTexture2D<unorm float4> back : register(u0);
cbuffer P : register(b0) { uint w; uint h; uint seed; uint pad; };
[numthreads(8, 8, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (id.x >= w || id.y >= h) return;
    uint r = (id.x * 7 + seed) & 255;
    uint g = (id.y * 3 + seed * 5) & 255;
    uint b = (id.x + id.y) & 255;
    back[id.xy] = float4(r, g, b, 255) / 255.0;
}
`

func texPattern(w, h, seed uint32) []byte {
	out := make([]byte, 0, 4*w*h)
	for y := uint32(0); y < h; y++ {
		for x := uint32(0); x < w; x++ {
			out = append(out, byte(x*7+seed), byte(y*3+seed*5), byte(x+y), 255)
		}
	}
	return out
}

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	procCreateWindow  = user32.NewProc("CreateWindowExW")
	procDestroyWindow = user32.NewProc("DestroyWindow")
)

// hiddenWindow makes an overlapped window that is never shown (no WS_VISIBLE,
// no ShowWindow): the swap chain needs an HWND, not a screen.
func hiddenWindow(t *testing.T, w, h int) uintptr {
	t.Helper()
	class, _ := windows.UTF16PtrFromString("STATIC") // a predefined window class
	title, _ := windows.UTF16PtrFromString("d3d11 swap chain test")
	const wsOverlappedWindow = 0x00CF0000
	r, _, err := procCreateWindow.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow, 0, 0, uintptr(w), uintptr(h), 0, 0, 0, 0)
	if r == 0 {
		t.Fatalf("CreateWindowExW: %v", err)
	}
	t.Cleanup(func() { procDestroyWindow.Call(r) })
	return r
}

// fillBackBuffer runs the pattern kernel into the swap chain's current back
// buffer and returns the buffer's pixels read back through staging, before
// Present (a discarded buffer is undefined after it).
func fillBackBuffer(t *testing.T, d *Device, sc *SwapChain, sh *ComputeShader, seed uint32) []byte {
	t.Helper()
	ctx := d.Context()
	w, h := sc.Size()
	bb, err := sc.BackBuffer()
	if err != nil {
		t.Fatal(err)
	}
	defer bb.Close()
	uav, err := bb.UAV()
	if err != nil {
		t.Fatal(err)
	}
	defer uav.Close()
	var p [16]byte
	binary.LittleEndian.PutUint32(p[0:], w)
	binary.LittleEndian.PutUint32(p[4:], h)
	binary.LittleEndian.PutUint32(p[8:], seed)
	cb := must(d.CreateBuffer(BufferDesc{Kind: KindConstant, Size: 16, Initial: p[:]}))
	for _, err := range []error{
		ctx.SetShader(sh), ctx.SetConstantBuffers(0, cb), ctx.SetUAVs(0, uav), ctx.Dispatch((w+7)/8, (h+7)/8, 1),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := ctx.ReadbackTexture(bb, waitFor)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetUAVs(0, nil); err != nil { // the view is a reference: unbind it
		t.Fatal(err)
	}
	return got
}

func texShader(t *testing.T, d *Device) *ComputeShader {
	t.Helper()
	dxbc, err := d3dcompiler.Compile([]byte(texKernel), "tex.hlsl", "main", "cs_5_0", d3dcompiler.FlagOptimizationLevel3|d3dcompiler.FlagWarningsAreErrors)
	if err != nil {
		t.Fatal(err)
	}
	return must(d.CreateComputeShader(dxbc))
}

// TestSwapChainBackBufferUAV is the D4 proof on WARP: a flip-model FLIP_DISCARD
// swap chain on a hidden window, its back buffer written by a compute shader as
// a UAV and read back through a staging texture byte for byte, over several
// presents (the back buffer is asked for again after each).
func TestSwapChainBackBufferUAV(t *testing.T) {
	d := newWARP(t)
	const w, h = 64, 48
	hwnd := hiddenWindow(t, w, h)
	sc, err := d.CreateSwapChain(hwnd, dxgi.SwapChainDesc{Width: w, Height: h})
	if err != nil {
		t.Fatalf("flip swap chain with UAV back buffers on WARP: %v", err)
	}
	defer sc.Close()
	sh := texShader(t, d)
	for frame := uint32(0); frame < 4; frame++ {
		got := fillBackBuffer(t, d, sc, sh, 11*frame+1)
		if want := texPattern(w, h, 11*frame+1); !bytes.Equal(got, want) {
			t.Fatalf("frame %d: the back buffer is not the kernel's pattern (first pixels % x, want % x)", frame, got[:8], want[:8])
		}
		if err := sc.Present(0); err != nil {
			t.Fatalf("Present %d: %v", frame, err)
		}
	}
}

// TestBreakCheckStaleBackBuffer: without the dispatch the read-back is not the
// pattern, so the comparison above tells a written buffer from an unwritten one.
func TestBreakCheckStaleBackBuffer(t *testing.T) {
	d := newWARP(t)
	const w, h = 32, 16
	sc, err := d.CreateSwapChain(hiddenWindow(t, w, h), dxgi.SwapChainDesc{Width: w, Height: h})
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()
	bb, err := sc.BackBuffer()
	if err != nil {
		t.Fatal(err)
	}
	defer bb.Close()
	got, err := d.Context().ReadbackTexture(bb, waitFor)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, texPattern(w, h, 1)) {
		t.Fatal("an unwritten back buffer equals the pattern: the check proves nothing")
	}
}

// TestSwapChainResize: ResizeBuffers fails while a back buffer is referenced
// (the documented rule, and the break check of this test), works once every
// reference is gone, and the resized buffers take the pattern at the new size.
func TestSwapChainResize(t *testing.T) {
	d := newWARP(t)
	hwnd := hiddenWindow(t, 64, 48)
	sc, err := d.CreateSwapChain(hwnd, dxgi.SwapChainDesc{Width: 64, Height: 48})
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()
	sh := texShader(t, d)
	bb, err := sc.BackBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sc.Resize(32, 24); err == nil {
		t.Fatal("ResizeBuffers succeeded with a reference to a back buffer alive")
	} else {
		t.Logf("with a back buffer referenced: %v", err)
	}
	if w, h := sc.Size(); w != 64 || h != 48 {
		t.Fatalf("a refused Resize changed the size to %d x %d", w, h)
	}
	bb.Close()
	if err := sc.Resize(32, 24); err != nil {
		t.Fatal(err)
	}
	if w, h := sc.Size(); w != 32 || h != 24 {
		t.Fatalf("size %d x %d after Resize(32, 24)", w, h)
	}
	if got := fillBackBuffer(t, d, sc, sh, 9); !bytes.Equal(got, texPattern(32, 24, 9)) {
		t.Fatal("the resized back buffer is not the pattern")
	}
	if err := sc.Present(0); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][2]uint32{{0, 8}, {8, 0}, {dxgi.MaxBackBufferSide + 1, 8}} {
		if err := sc.Resize(bad[0], bad[1]); !errors.Is(err, ErrBadDesc) {
			t.Errorf("Resize(%d, %d): %v", bad[0], bad[1], err)
		}
	}
}

func TestSwapChainRefusals(t *testing.T) {
	d := newWARP(t)
	if _, err := d.CreateSwapChain(hiddenWindow(t, 8, 8), dxgi.SwapChainDesc{Width: 8}); !errors.Is(err, dxgi.ErrBadDesc) {
		t.Errorf("a description without a height: %v", err)
	}
	if _, err := d.CreateSwapChain(0, dxgi.SwapChainDesc{Width: 8, Height: 8}); err == nil {
		t.Error("a swap chain without a window")
	}
	var sc *SwapChain
	if err := sc.Present(0); !errors.Is(err, ErrClosed) {
		t.Errorf("Present on nil: %v", err)
	}
	if _, err := sc.BackBuffer(); !errors.Is(err, ErrClosed) {
		t.Errorf("BackBuffer on nil: %v", err)
	}
	if err := sc.Resize(8, 8); !errors.Is(err, ErrClosed) {
		t.Errorf("Resize on nil: %v", err)
	}
	var tex *Texture
	if _, err := tex.UAV(); !errors.Is(err, ErrClosed) {
		t.Errorf("UAV on nil: %v", err)
	}
	if _, err := d.Context().ReadbackTexture(nil, waitFor); !errors.Is(err, ErrClosed) {
		t.Errorf("ReadbackTexture of nil: %v", err)
	}
	sc.Close()
	tex.Close()
}
