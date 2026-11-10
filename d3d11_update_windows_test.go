// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package d3d11

import (
	"bytes"
	"errors"
	"testing"
)

// TestUpdateBufferDefault: UpdateSubresource on a DEFAULT structured buffer
// with a UAV, whole and partial (a box), read back with Readback.
func TestUpdateBufferDefault(t *testing.T) {
	d := newWARP(t)
	ctx := d.Context()
	buf := must(d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: 64, Stride: 4, UAV: true}))
	a := make([]uint32, 16)
	for i := range a {
		a[i] = 100 + uint32(i)
	}
	if err := ctx.UpdateBuffer(buf, u32bytes(a)); err != nil {
		t.Fatal(err)
	}
	got, err := ctx.Readback(buf, waitFor)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, u32bytes(a)) {
		t.Fatalf("whole update: %x", words(got))
	}
	// A partial update writes the first 5 words and leaves the other 11.
	if err := ctx.UpdateBuffer(buf, u32bytes([]uint32{1, 2, 3, 4, 5})); err != nil {
		t.Fatal(err)
	}
	copy(a, []uint32{1, 2, 3, 4, 5})
	if got, err = ctx.Readback(buf, waitFor); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, u32bytes(a)) {
		t.Fatalf("partial update: %x, want %x", words(got), a)
	}
}

// copyThrough runs the "oob" kernel with value 0, dst[i] = src[i], from a
// structured SRV at t0 to a fresh structured UAV at u0, and returns dst.
func copyThrough(t *testing.T, d *Device, src *Buffer, n uint32) []uint32 {
	t.Helper()
	ctx := d.Context()
	dst := must(d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: src.Size(), Stride: 4, UAV: true}))
	cb := must(d.CreateBuffer(BufferDesc{Kind: KindConstant, Size: 16, Initial: params(n, 0)}))
	for _, err := range []error{
		ctx.SetShader(kernel(t, d, "oob")), ctx.SetConstantBuffers(0, cb),
		ctx.SetSRVs(0, must(src.SRV())), ctx.SetUAVs(0, must(dst.UAV())),
		ctx.Dispatch((n+63)/64, 1, 1),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	out, err := ctx.Readback(dst, waitFor)
	if err != nil {
		t.Fatal(err)
	}
	return words(out)
}

// TestUpdateBufferDynamicFeedsAShader: one DYNAMIC structured buffer, rewritten
// by UpdateBuffer (Map WRITE_DISCARD) every round and read by a compute shader
// through an SRV, as a renderer reuses a frame's input buffer. Each round must
// see its own data and none of the previous round's.
func TestUpdateBufferDynamicFeedsAShader(t *testing.T) {
	d := newWARP(t)
	const n = 100
	src := must(d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: 4 * 128, Stride: 4, SRV: true, Dynamic: true}))
	for round := range uint32(5) {
		in := make([]uint32, n)
		for i := range in {
			in[i] = round*1000 + uint32(i)
		}
		if err := d.Context().UpdateBuffer(src, u32bytes(in)); err != nil { // 100 of 128 words
			t.Fatal(err)
		}
		got := copyThrough(t, d, src, n)
		for i := range in {
			if got[i] != in[i] {
				t.Fatalf("round %d word %d = %d, want %d", round, i, got[i], in[i])
			}
		}
	}
}

// TestBreakCheckStaleBuffer: the same loop without the UpdateBuffer of round 1
// must read round 0's data: the comparison of the test above tells fresh data
// from stale, so that test can fail.
func TestBreakCheckStaleBuffer(t *testing.T) {
	d := newWARP(t)
	const n = 64
	src := must(d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: 4 * n, Stride: 4, SRV: true, Dynamic: true}))
	round := func(v uint32, update bool) []uint32 {
		in := make([]uint32, n)
		for i := range in {
			in[i] = v + uint32(i)
		}
		if update {
			if err := d.Context().UpdateBuffer(src, u32bytes(in)); err != nil {
				t.Fatal(err)
			}
		}
		return copyThrough(t, d, src, n)
	}
	round(1000, true)
	got := round(2000, false)
	if got[0] == 2000 || got[0] != 1000 {
		t.Fatalf("without the update the shader read %d, want the stale 1000", got[0])
	}
}

// TestUpdateRawDynamic: a raw dynamic buffer, through the rawadd kernel.
func TestUpdateRawDynamic(t *testing.T) {
	d := newWARP(t)
	ctx := d.Context()
	src := must(d.CreateBuffer(BufferDesc{Kind: KindRaw, Size: 256, SRV: true, Dynamic: true, Initial: make([]byte, 256)}))
	dst := must(d.CreateBuffer(BufferDesc{Kind: KindRaw, Size: 256, UAV: true}))
	cb := must(d.CreateBuffer(BufferDesc{Kind: KindConstant, Size: 16, Initial: params(64, 7)}))
	in := make([]uint32, 64)
	for i := range in {
		in[i] = uint32(i) * 5
	}
	if err := ctx.UpdateBuffer(src, u32bytes(in)); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{
		ctx.SetShader(kernel(t, d, "rawadd")), ctx.SetConstantBuffers(0, cb),
		ctx.SetSRVs(1, must(src.SRV())), ctx.SetUAVs(1, must(dst.UAV())), ctx.Dispatch(1, 1, 1),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	out, err := ctx.Readback(dst, waitFor)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range words(out) {
		if v != in[i]+7 {
			t.Fatalf("word %d = %d, want %d", i, v, in[i]+7)
		}
	}
}

func TestDynamicDescRefusals(t *testing.T) {
	d := newWARP(t)
	for name, desc := range map[string]BufferDesc{
		"with a UAV":  {Kind: KindStructured, Size: 64, Stride: 4, SRV: true, UAV: true, Dynamic: true},
		"constant":    {Kind: KindConstant, Size: 16, Dynamic: true},
		"staging":     {Kind: KindStaging, Size: 64, Dynamic: true},
		"without SRV": {Kind: KindRaw, Size: 64, Dynamic: true},
	} {
		if _, err := d.CreateBuffer(desc); !errors.Is(err, ErrBadDesc) {
			t.Errorf("dynamic buffer %s: %v", name, err)
		}
	}
}

// TestFenceWaitsForItsOwnFrame: two frames of work on the one context, a fence
// after each. The first fence's wait must leave the first frame's result
// complete in its staging copy, with the second frame still queued behind it.
func TestFenceWaitsForItsOwnFrame(t *testing.T) {
	d := newWARP(t)
	ctx := d.Context()
	const total, count = 1024, 1000
	dst := must(d.CreateBuffer(BufferDesc{Kind: KindStructured, Size: total * 4, Stride: 4, UAV: true}))
	uav := must(dst.UAV())
	cb := must(d.CreateBuffer(BufferDesc{Kind: KindConstant, Size: 16}))
	sh := kernel(t, d, "fill")
	var fences [2]*Fence
	var stage [2]*Buffer
	for f := range 2 {
		base := uint32(f+1) * 0x10000
		stage[f] = must(d.CreateBuffer(BufferDesc{Kind: KindStaging, Size: total * 4}))
		for _, err := range []error{
			ctx.UpdateBuffer(cb, params(count, base)), ctx.SetShader(sh), ctx.SetConstantBuffers(0, cb),
			ctx.SetUAVs(0, uav), ctx.Dispatch((count+63)/64, 1, 1), ctx.CopyBuffer(stage[f], dst),
		} {
			if err != nil {
				t.Fatal(err)
			}
		}
		fences[f] = must(ctx.Signal())
	}
	if err := ctx.Flush(); err != nil {
		t.Fatal(err)
	}
	for f := range 2 {
		if err := fences[f].Wait(waitFor); err != nil {
			t.Fatal(err)
		}
		if done, err := fences[f].Done(); err != nil || !done {
			t.Fatalf("fence %d after Wait: %v %v", f, done, err)
		}
		m, err := ctx.MapRead(stage[f])
		if err != nil {
			t.Fatal(err)
		}
		w := words(append([]byte(nil), m...))
		if err := ctx.Unmap(stage[f]); err != nil {
			t.Fatal(err)
		}
		base := uint32(f+1) * 0x10000
		for i := 0; i < count; i++ {
			if w[i] != base+uint32(i) {
				t.Fatalf("frame %d word %d = %#x, want %#x (fence returned before its frame was done)", f, i, w[i], base+uint32(i))
			}
		}
	}
	fences[0].Close()
	if _, err := fences[0].Done(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Done after Close: %v", err)
	}
}
