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
