// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

package dxgi

import (
	"errors"
	"testing"
)

func TestAdapterDesc(t *testing.T) {
	if (AdapterDesc{}).Software() || !(AdapterDesc{Flags: AdapterFlagSoftware}).Software() {
		t.Fatal("Software flag")
	}
	if AdapterFlagSoftware != 2 { // DXGI_ADAPTER_FLAG_SOFTWARE
		t.Fatal("flag value")
	}
}

func TestSwapChainDescValidated(t *testing.T) {
	d, err := SwapChainDesc{Width: 8, Height: 4}.validated()
	if err != nil || d.BufferCount != 2 {
		t.Fatalf("defaults: %+v %v", d, err)
	}
	if d, err = (SwapChainDesc{Width: MaxBackBufferSide, Height: 1, BufferCount: MaxSwapChainBuffers}).validated(); err != nil || d.BufferCount != 16 {
		t.Fatalf("limits: %+v %v", d, err)
	}
	for _, bad := range []SwapChainDesc{
		{}, {Width: 1}, {Height: 1}, {Width: MaxBackBufferSide + 1, Height: 1}, {Width: 1, Height: MaxBackBufferSide + 1},
		{Width: 1, Height: 1, BufferCount: 1}, {Width: 1, Height: 1, BufferCount: MaxSwapChainBuffers + 1},
	} {
		if _, err := bad.validated(); !errors.Is(err, ErrBadDesc) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}
