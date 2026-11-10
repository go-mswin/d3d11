// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build !windows

package d3dcompiler

// Compile is unavailable off Windows.
func Compile(src []byte, sourceName, entry, target string, flags uint32) ([]byte, error) {
	return nil, ErrUnsupportedPlatform
}
