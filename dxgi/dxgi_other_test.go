// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build !windows

package dxgi

import (
	"errors"
	"testing"
)

func TestStubs(t *testing.T) {
	f, err := CreateFactory1()
	if f != nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(f, err)
	}
	f = &Factory{}
	if a, err := f.Adapters(); a != nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(a, err)
	}
	a, err := f.EnumAdapter(0)
	if a != nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(a, err)
	}
	f.Close()
	a = &Adapter{}
	if _, err := a.Desc(); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(err)
	}
	if a.Ptr() != nil {
		t.Fatal("Ptr")
	}
	a.Close()
	if _, err := DescribeDeviceAdapter(nil); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(err)
	}
}

func TestSwapChainStubs(t *testing.T) {
	if _, err := FactoryOfDevice(nil); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(err)
	}
	f := &Factory{}
	if _, err := f.CreateSwapChainForHwnd(nil, 1, SwapChainDesc{}); !errors.Is(err, ErrBadDesc) {
		t.Fatal(err)
	}
	if _, err := f.CreateSwapChainForHwnd(nil, 1, SwapChainDesc{Width: 8, Height: 8}); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(err)
	}
	s := &SwapChain{}
	if err := s.Present(0); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(err)
	}
	if p, err := s.BackBuffer(0); p != nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(p, err)
	}
	if err := s.ResizeBuffers(8, 8); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(err)
	}
	s.Close()
}
