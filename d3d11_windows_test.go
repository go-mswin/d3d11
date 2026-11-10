// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package d3d11

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-mswin/d3d11/d3dcompiler"
	"github.com/go-mswin/d3d11/internal/com"
)

const waitFor = 30 * time.Second

var kernelEntries = []string{"fill", "rawadd", "oob"}

// newWARP creates a WARP device, and fails the test (never skips) if it
// cannot: WARP is part of every Windows 10 and later, and it is the proof.
func newWARP(t *testing.T) *Device {
	t.Helper()
	curT = t
	before := com.Live()
	t.Cleanup(func() {
		if got := com.Live(); got != before {
			t.Errorf("%d COM object(s) leaked", got-before)
		}
	})
	d, err := CreateDevice(Options{Software: true})
	if err != nil {
		t.Fatalf("WARP device: %v", err)
	}
	t.Cleanup(d.Close)
	if d.FeatureLevel() < FeatureLevel11_0 {
		t.Fatalf("feature level %s", d.FeatureLevel())
	}
	t.Logf("adapter %q software=%v feature level %s max buffer %d MiB",
		d.Adapter().Description, d.Adapter().Software(), d.FeatureLevel(), d.MaxBufferBytes()>>20)
	return d
}

func kernel(t *testing.T, d *Device, entry string) *ComputeShader {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", entry+".dxbc"))
	if err != nil {
		t.Fatalf("committed DXBC: %v (run TestDXBCNotDrifted with D3D11_DXBC_OUT set on Windows)", err)
	}
	s, err := d.CreateComputeShader(b)
	if err != nil {
		t.Fatalf("CreateComputeShader(%s): %v", entry, err)
	}
	t.Cleanup(s.Close)
	return s
}

func params(count, value uint32) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b[0:], count)
	binary.LittleEndian.PutUint32(b[4:], value)
	return b
}

func words(b []byte) []uint32 {
	w := make([]uint32, len(b)/4)
	for i := range w {
		w[i] = binary.LittleEndian.Uint32(b[4*i:])
	}
	return w
}

func u32bytes(w []uint32) []byte {
	b := make([]byte, 4*len(w))
	for i, v := range w {
		binary.LittleEndian.PutUint32(b[4*i:], v)
	}
	return b
}

// curT is the test running now; the tests here are not parallel. It lets must
// take the (value, error) pair of a constructor directly.
var curT *testing.T

// must returns v, closing it when the test ends, or fails the test.
func must[T interface{ Close() }](v T, err error) T {
	curT.Helper()
	if err != nil {
		curT.Fatal(err)
	}
	curT.Cleanup(v.Close)
	return v
}

// runFill dispatches the fill kernel into a 1024-element structured buffer with
// the UAV bound at slot uavSlot, and returns the read-back words.
func runFill(t *testing.T, d *Device, uavSlot uint32) []uint32 {
	t.Helper()
	const total, count, value = 1024, 1000, 0xA5000000
	ctx := d.Context()
	dst := must(d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: total * 4, Stride: 4, UAV: true}))
	uav := must(dst.UAV())
	cb := must(d.CreateBuffer(BufferDesc{Kind: KindConstant, Size: 16, Initial: params(count, value)}))
	if err := ctx.SetShader(kernel(t, d, "fill")); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetConstantBuffers(0, cb); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetUAVs(uavSlot, uav); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Dispatch((count+63)/64, 1, 1); err != nil {
		t.Fatal(err)
	}
	out, err := ctx.Readback(dst, waitFor)
	if err != nil {
		t.Fatal(err)
	}
	return words(out)
}

func fillExpected() []byte {
	w := make([]uint32, 1024)
	for i := 0; i < 1000; i++ { // the 24 elements past count stay zero
		w[i] = 0xA5000000 + uint32(i)
	}
	return u32bytes(w)
}

// The D1 proof: a WARP device dispatches the fill shader and the staging
// read-back has the expected bytes.
func TestWARPFillDispatch(t *testing.T) {
	d := newWARP(t)
	got := u32bytes(runFill(t, d, 0))
	if want := fillExpected(); !bytes.Equal(got, want) {
		t.Fatalf("read-back differs from the expected fill (first words %x vs %x)", words(got)[:4], words(want)[:4])
	}
}

// The break check: a wrong UAV binding must fail. The shader writes u0; with
// the view bound at u1 nothing is bound at u0, the writes are dropped and the
// buffer stays zero, so the same comparison as above must NOT pass.
func TestWrongUAVBindingFails(t *testing.T) {
	d := newWARP(t)
	got := u32bytes(runFill(t, d, 1))
	if bytes.Equal(got, fillExpected()) {
		t.Fatal("a UAV bound at the wrong slot still produced the expected bytes: the check proves nothing")
	}
	if !bytes.Equal(got, make([]byte, len(got))) {
		t.Fatal("expected an untouched (zero) buffer when the UAV is bound at the wrong slot")
	}
}

func TestWARPRawAdd(t *testing.T) {
	d := newWARP(t)
	ctx := d.Context()
	const n, add = 200, 7
	in := make([]uint32, 256)
	for i := range in {
		in[i] = uint32(i) * 3
	}
	src := must(d.CreateBuffer(BufferDesc{Kind: KindRaw, Size: 256 * 4, SRV: true, Initial: u32bytes(in)}))
	dst := must(d.CreateBuffer(BufferDesc{Kind: KindRaw, Size: 256 * 4, UAV: true}))
	cb := must(d.CreateBuffer(BufferDesc{Kind: KindConstant, Size: 16, Initial: params(n, add)}))
	srv, uav := must(src.SRV()), must(dst.UAV())
	for _, err := range []error{
		ctx.SetShader(kernel(t, d, "rawadd")), ctx.SetConstantBuffers(0, cb),
		ctx.SetSRVs(1, srv), ctx.SetUAVs(1, uav), ctx.Dispatch(4, 1, 1),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	out, err := ctx.Readback(dst, waitFor)
	if err != nil {
		t.Fatal(err)
	}
	got := words(out)
	for i := range got {
		want := uint32(0)
		if i < n {
			want = in[i] + add
		}
		if got[i] != want {
			t.Fatalf("word %d = %d, want %d", i, got[i], want)
		}
	}
}

// What an out-of-bounds read of a structured buffer returns, measured on WARP.
// The spec (ld_structured): out-of-bounds addressing on t# returns 0 for each
// 32-bit component, except that an out-of-bounds srcByteOffset is undefined.
// Here the struct INDEX is out of bounds, so 0 is what the spec promises.
func TestWARPStructuredOutOfBoundsReadsZero(t *testing.T) {
	d := newWARP(t)
	ctx := d.Context()
	const elems, shift = 16, 8
	in := make([]uint32, elems)
	for i := range in {
		in[i] = uint32(i) + 1
	}
	src := must(d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: elems * 4, Stride: 4, SRV: true, Initial: u32bytes(in)}))
	dst := must(d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: elems * 4, Stride: 4, UAV: true}))
	cb := must(d.CreateBuffer(BufferDesc{Kind: KindConstant, Size: 16, Initial: params(elems, shift)}))
	srv, uav := must(src.SRV()), must(dst.UAV())
	for _, err := range []error{
		ctx.SetShader(kernel(t, d, "oob")), ctx.SetConstantBuffers(0, cb),
		ctx.SetSRVs(0, srv), ctx.SetUAVs(0, uav), ctx.Dispatch(1, 1, 1),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	out, err := ctx.Readback(dst, waitFor)
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range words(out) {
		want := uint32(0)
		if i+shift < elems {
			want = in[i+shift]
		}
		if g != want {
			t.Fatalf("dst[%d] = %d, want %d (out-of-bounds reads must be 0)", i, g, want)
		}
	}
}

func TestBufferSizeLimit(t *testing.T) {
	d := newWARP(t)
	max := d.MaxBufferBytes()
	if max != 128<<20 {
		t.Logf("WARP reports a limit of %d bytes, the guaranteed minimum is %d", max, 128<<20)
	}
	// One element past the limit is refused before any COM call.
	before := com.Live()
	_, err := d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: max + 4, Stride: 4, UAV: true})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v, want ErrTooLarge", err)
	}
	if com.Live() != before {
		t.Fatal("a refused size created something")
	}
	// Exactly the limit is accepted.
	b, err := d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: max, Stride: 4, UAV: true})
	if err != nil {
		t.Fatalf("a buffer of exactly the documented limit (%d bytes): %v", max, err)
	}
	b.Close()
}

// Every error path leaves nothing alive (newWARP's cleanup checks the count).
func TestErrorPathsRelease(t *testing.T) {
	d := newWARP(t)
	ctx := d.Context()
	structured := must(d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: 64, Stride: 4, SRV: true}))
	if _, err := structured.UAV(); !errors.Is(err, ErrBadDesc) {
		t.Errorf("UAV without the flag: %v", err)
	}
	staging := must(d.CreateBuffer(BufferDesc{Kind: KindStaging, Size: 64}))
	if _, err := ctx.MapRead(structured); !errors.Is(err, ErrBadDesc) {
		t.Errorf("MapRead of a non-staging buffer: %v", err)
	}
	if err := ctx.Unmap(staging); !errors.Is(err, ErrMapped) {
		t.Errorf("Unmap without Map: %v", err)
	}
	m, err := ctx.MapRead(staging)
	if err != nil || len(m) != 64 {
		t.Fatalf("MapRead: %d %v", len(m), err)
	}
	if _, err := ctx.MapRead(staging); !errors.Is(err, ErrMapped) {
		t.Errorf("double Map: %v", err)
	}
	if err := ctx.Unmap(staging); err != nil {
		t.Error(err)
	}
	// DXBC magic and header size, garbage after: D3D itself refuses it.
	junk := append([]byte("DXBC"), make([]byte, 60)...)
	if s, err := d.CreateComputeShader(junk); err == nil {
		s.Close()
		t.Error("garbage bytecode accepted")
	}
	if _, err := d.CreateComputeShader([]byte("nope")); !errors.Is(err, ErrBadBytecode) {
		t.Errorf("short bytecode: %v", err)
	}
	if err := ctx.Dispatch(MaxDispatchGroups+1, 1, 1); !errors.Is(err, ErrBadDesc) {
		t.Errorf("dispatch: %v", err)
	}
	if err := ctx.SetUAVs(MaxUAVSlots, nil); !errors.Is(err, ErrBadDesc) {
		t.Errorf("UAV slot: %v", err)
	}
	if err := ctx.SetUAVs(0); !errors.Is(err, ErrBadDesc) {
		t.Errorf("no UAVs: %v", err)
	}
	if err := ctx.SetSRVs(MaxSRVSlots, nil); !errors.Is(err, ErrBadDesc) {
		t.Errorf("SRV slot: %v", err)
	}
	if err := ctx.SetConstantBuffers(0, structured); !errors.Is(err, ErrBadDesc) {
		t.Errorf("non-constant buffer in a constant slot: %v", err)
	}
	if err := ctx.UpdateBuffer(structured, make([]byte, 64)); !errors.Is(err, ErrBadDesc) {
		t.Errorf("UpdateBuffer of a non-constant buffer: %v", err)
	}
	cb := must(d.CreateBuffer(BufferDesc{Kind: KindConstant, Size: 16}))
	if err := ctx.UpdateBuffer(cb, make([]byte, 8)); !errors.Is(err, ErrBadDesc) {
		t.Errorf("short UpdateBuffer: %v", err)
	}
	if err := ctx.CopyBuffer(staging, cb); !errors.Is(err, ErrBadDesc) {
		t.Errorf("copy of different sizes: %v", err)
	}
	// A raised or lowered limit is honoured.
	small, err := CreateDevice(Options{Software: true, MaxBufferBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer small.Close()
	if _, err := small.CreateBuffer(BufferDesc{Kind: KindRaw, Size: 2 << 20, UAV: true}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("override limit: %v", err)
	}
}

func TestClosedObjects(t *testing.T) {
	before := com.Live()
	d, err := CreateDevice(Options{Software: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := d.Context()
	b, err := d.CreateBuffer(BufferDesc{Kind: KindRaw, Size: 16, UAV: true})
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	d.Close() // idempotent
	if _, err := d.CreateBuffer(BufferDesc{Kind: KindRaw, Size: 16, UAV: true}); !errors.Is(err, ErrClosed) {
		t.Errorf("CreateBuffer after Close: %v", err)
	}
	if err := ctx.Dispatch(1, 1, 1); !errors.Is(err, ErrClosed) {
		t.Errorf("Dispatch after Close: %v", err)
	}
	if err := d.RemovedReason(); !errors.Is(err, ErrClosed) {
		t.Errorf("RemovedReason after Close: %v", err)
	}
	b.Close()
	b.Close()
	if _, err := b.UAV(); !errors.Is(err, ErrClosed) {
		t.Errorf("UAV of a closed buffer: %v", err)
	}
	if got := com.Live(); got != before {
		t.Fatalf("%d COM object(s) leaked", got-before)
	}
}

func TestWARPNotRemoved(t *testing.T) {
	d := newWARP(t)
	if err := d.RemovedReason(); err != nil {
		t.Fatal(err)
	}
}

// TestDXBCNotDrifted is the drift test: it recompiles testdata/kernels.hlsl with
// the system D3DCompile (cs_5_0) and compares with the committed
// testdata/<entry>.dxbc. With D3D11_DXBC_OUT=<dir> set it also writes the
// fresh bytes there (CI uploads them as an artifact), which is how the files
// are regenerated: copy them over testdata/ and commit.
func TestDXBCNotDrifted(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "kernels.hlsl"))
	if err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("D3D11_DXBC_OUT")
	for _, entry := range kernelEntries {
		fresh, err := d3dcompiler.Compile(src, "kernels.hlsl", entry, "cs_5_0",
			d3dcompiler.FlagOptimizationLevel3|d3dcompiler.FlagWarningsAreErrors)
		if err != nil {
			t.Fatalf("%s: %v", entry, err)
		}
		if err := checkDXBC(fresh); err != nil {
			t.Fatalf("%s: compiler output: %v", entry, err)
		}
		if out != "" {
			if err := os.MkdirAll(out, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, entry+".dxbc"), fresh, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		committed, err := os.ReadFile(filepath.Join("testdata", entry+".dxbc"))
		if err != nil {
			t.Errorf("%s: no committed DXBC: %v", entry, err)
			continue
		}
		if !bytes.Equal(committed, fresh) {
			t.Errorf("%s: testdata/%s.dxbc (%d bytes) differs from a fresh compile (%d bytes); regenerate with D3D11_DXBC_OUT",
				entry, entry, len(committed), len(fresh))
		}
	}
}

func TestCompileErrors(t *testing.T) {
	if _, err := d3dcompiler.Compile(nil, "x", "main", "cs_5_0", 0); !errors.Is(err, d3dcompiler.ErrCompile) {
		t.Errorf("empty source: %v", err)
	}
	_, err := d3dcompiler.Compile([]byte("this is not hlsl"), "bad.hlsl", "main", "cs_5_0", 0)
	if !errors.Is(err, d3dcompiler.ErrCompile) {
		t.Fatalf("garbage source: %v", err)
	}
	t.Log(err)
}
