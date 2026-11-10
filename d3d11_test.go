// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

package d3d11

import (
	"errors"
	"math"
	"testing"
)

func TestMaxBufferBytes(t *testing.T) {
	const mb = 1 << 20
	const gb = 1 << 30
	for _, c := range []struct{ vram, want uint64 }{
		{0, 128 * mb},        // WARP, software adapters
		{256 * mb, 128 * mb}, // 0.25 * 256 = 64 < 128
		{512 * mb, 128 * mb}, // exactly A
		{1 * gb, 256 * mb},
		{8 * gb, 2048 * mb}, // 0.25 * 8 GB = 2 GB = C
		{16 * gb, 2048 * mb},
		{math.MaxUint64, 2048 * mb},
	} {
		if got := MaxBufferBytes(c.vram); got != c.want {
			t.Errorf("MaxBufferBytes(%d) = %d, want %d", c.vram, got, c.want)
		}
	}
}

func TestElementBytesNoWrap(t *testing.T) {
	if got := ElementBytes(math.MaxUint32, math.MaxUint32); got != uint64(math.MaxUint32)*uint64(math.MaxUint32) {
		t.Fatal(got)
	}
	// 2^20 elements of 4096 bytes wraps a uint32 to 0; it must not here.
	if ElementBytes(1<<20, 4096) != 1<<32 {
		t.Fatal("wrapped")
	}
}

func TestValidate(t *testing.T) {
	const lim = 1 << 20
	ok := []BufferDesc{
		{Kind: KindStructured, Size: 64, Stride: 4, SRV: true, UAV: true},
		{Kind: KindStructured, Size: 2048, Stride: 2048, SRV: true, Initial: make([]byte, 2048)},
		{Kind: KindRaw, Size: 16, UAV: true},
		{Kind: KindConstant, Size: 16},
		{Kind: KindConstant, Size: MaxConstantBufferBytes},
		{Kind: KindStaging, Size: lim},
	}
	for i, d := range ok {
		if w, err := d.validate(lim); err != nil || uint64(w) != d.Size {
			t.Errorf("ok[%d]: %d %v", i, w, err)
		}
	}
	bad := []struct {
		d    BufferDesc
		want error
	}{
		{BufferDesc{Kind: KindRaw, UAV: true}, ErrBadDesc},
		{BufferDesc{Kind: KindRaw, Size: lim + 4, UAV: true}, ErrTooLarge},
		{BufferDesc{Kind: KindRaw, Size: 6, UAV: true}, ErrBadDesc},
		{BufferDesc{Kind: KindRaw, Size: 8, Stride: 4, UAV: true}, ErrBadDesc},
		{BufferDesc{Kind: KindRaw, Size: 8, UAV: true, Initial: make([]byte, 4)}, ErrBadDesc},
		{BufferDesc{Kind: KindStructured, Size: 8, UAV: true}, ErrBadDesc},
		{BufferDesc{Kind: KindStructured, Size: 8, Stride: 6, UAV: true}, ErrBadDesc},
		{BufferDesc{Kind: KindStructured, Size: 8192, Stride: 4096, UAV: true}, ErrBadDesc},
		{BufferDesc{Kind: KindStructured, Size: 10, Stride: 4, UAV: true}, ErrBadDesc},
		{BufferDesc{Kind: KindConstant, Size: 8}, ErrBadDesc},
		{BufferDesc{Kind: KindConstant, Size: MaxConstantBufferBytes + 16}, ErrBadDesc},
		{BufferDesc{Kind: KindConstant, Size: 16, UAV: true}, ErrBadDesc},
		{BufferDesc{Kind: KindStaging, Size: 16, SRV: true}, ErrBadDesc},
		{BufferDesc{Kind: KindStaging, Size: 16, Stride: 4}, ErrBadDesc},
		{BufferDesc{Kind: KindRaw, Size: 16}, ErrBadDesc},
		{BufferDesc{Kind: KindStructured, Size: 16, Stride: 4}, ErrBadDesc},
		{BufferDesc{Kind: 0, Size: 16, SRV: true}, ErrBadDesc},
	}
	for i, c := range bad {
		if _, err := c.d.validate(lim); !errors.Is(err, c.want) {
			t.Errorf("bad[%d]: %v, want %v", i, err, c.want)
		}
	}
	// 32-bit overflow: a raised limit must still refuse a size beyond ByteWidth.
	if _, err := (BufferDesc{Kind: KindRaw, Size: 1 << 32, UAV: true}).validate(1 << 40); !errors.Is(err, ErrTooLarge) {
		t.Errorf("1<<32: %v", err)
	}
	// The largest documented size fits a UINT.
	if MaxBufferBytes(math.MaxUint64) > math.MaxUint32 {
		t.Error("the documented maximum no longer fits ByteWidth")
	}
}

func TestCheckDXBC(t *testing.T) {
	good := append([]byte("DXBC"), make([]byte, 28)...)
	if checkDXBC(good) != nil {
		t.Fatal("good")
	}
	for _, b := range [][]byte{nil, []byte("DXBC"), append([]byte("DXBD"), make([]byte, 28)...)} {
		if !errors.Is(checkDXBC(b), ErrBadBytecode) {
			t.Errorf("%q accepted", b)
		}
	}
}

func TestFeatureLevelString(t *testing.T) {
	for l, s := range map[FeatureLevel]string{
		FeatureLevel10_0: "10_0", FeatureLevel10_1: "10_1",
		FeatureLevel11_0: "11_0", FeatureLevel11_1: "11_1",
	} {
		if l.String() != s {
			t.Errorf("%#x = %s", uint32(l), l)
		}
	}
}
