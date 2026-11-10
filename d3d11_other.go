// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build !windows

package d3d11

// Device is an ID3D11Device with its immediate context. Off Windows it cannot
// be created.
type Device struct{}

// CreateDevice is unavailable off Windows.
func CreateDevice(Options) (*Device, error) { return nil, ErrUnsupportedPlatform }

// Close does nothing.
func (*Device) Close() {}
