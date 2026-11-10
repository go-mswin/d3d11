// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package d3dcompiler compiles HLSL to DXBC with D3DCompile from the system's
// d3dcompiler_47.dll, through syscall (no cgo, no SDK). It is how the DXBC a
// consumer commits is produced, and how the drift test checks the committed
// bytes: both run on the Windows CI lane. Off Windows, [Compile] returns
// [ErrUnsupportedPlatform].
package d3dcompiler

import "errors"

// ErrUnsupportedPlatform is returned off Windows.
var ErrUnsupportedPlatform = errors.New("d3dcompiler: only available on Windows")

// ErrCompile wraps the compiler's own message.
var ErrCompile = errors.New("d3dcompiler: HLSL compilation failed")

// Compile flags (D3DCOMPILE_*).
const (
	FlagOptimizationLevel3 = 1 << 15
	FlagWarningsAreErrors  = 1 << 18
	FlagIEEEStrictness     = 1 << 13
)

// Version is the DLL this package binds.
const Version = "d3dcompiler_47.dll"
