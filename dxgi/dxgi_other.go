// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build !windows

package dxgi

import "unsafe"

// Factory is an IDXGIFactory1. Off Windows it cannot be created.
type Factory struct{}

// Adapter is an IDXGIAdapter1. Off Windows it cannot be created.
type Adapter struct{}

// CreateFactory1 is unavailable off Windows.
func CreateFactory1() (*Factory, error) { return nil, ErrUnsupportedPlatform }

// EnumAdapter is unavailable off Windows.
func (*Factory) EnumAdapter(int) (*Adapter, error) { return nil, ErrUnsupportedPlatform }

// Adapters is unavailable off Windows.
func (*Factory) Adapters() ([]AdapterDesc, error) { return nil, ErrUnsupportedPlatform }

// Close does nothing.
func (*Factory) Close() {}

// Desc is unavailable off Windows.
func (*Adapter) Desc() (AdapterDesc, error) { return AdapterDesc{}, ErrUnsupportedPlatform }

// Ptr is nil off Windows.
func (*Adapter) Ptr() unsafe.Pointer { return nil }

// Close does nothing.
func (*Adapter) Close() {}

// DescribeDeviceAdapter is unavailable off Windows.
func DescribeDeviceAdapter(unsafe.Pointer) (AdapterDesc, error) {
	return AdapterDesc{}, ErrUnsupportedPlatform
}
