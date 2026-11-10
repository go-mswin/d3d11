// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package d3dcompiler

import (
	"fmt"
	"unsafe"

	"github.com/go-mswin/d3d11/internal/com"
	"golang.org/x/sys/windows"
)

var (
	modCompiler  = windows.NewLazySystemDLL(Version)
	procCompile  = modCompiler.NewProc("D3DCompile")
	errBlobEmpty = fmt.Errorf("%w: empty result", ErrCompile)
)

// ID3DBlob: IUnknown QueryInterface(0) AddRef(1) Release(2),
// GetBufferPointer(3) GetBufferSize(4).
const (
	slotBlobGetBufferPointer = 3
	slotBlobGetBufferSize    = 4
)

// blobBytes copies a blob's contents and releases the blob.
func blobBytes(b unsafe.Pointer) []byte {
	defer com.Release(b)
	n := com.CallSize(b, slotBlobGetBufferSize)
	p := com.CallPtr(b, slotBlobGetBufferPointer)
	if n == 0 || p == nil {
		return nil
	}
	return append([]byte(nil), unsafe.Slice((*byte)(p), int(n))...)
}

// Compile compiles src, entry point entry, for target (for example "cs_5_0").
// sourceName only labels messages. flags are D3DCOMPILE_* bits. On failure the
// compiler's message is in the error; every blob is released on every path.
func Compile(src []byte, sourceName, entry, target string, flags uint32) ([]byte, error) {
	if len(src) == 0 {
		return nil, fmt.Errorf("%w: empty source", ErrCompile)
	}
	if err := procCompile.Find(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCompile, err)
	}
	name, err := windows.BytePtrFromString(sourceName)
	if err != nil {
		return nil, err
	}
	ep, err := windows.BytePtrFromString(entry)
	if err != nil {
		return nil, err
	}
	tg, err := windows.BytePtrFromString(target)
	if err != nil {
		return nil, err
	}
	var code, msgs unsafe.Pointer
	r, _, _ := procCompile.Call(
		uintptr(unsafe.Pointer(&src[0])), uintptr(len(src)),
		uintptr(unsafe.Pointer(name)), 0, 0,
		uintptr(unsafe.Pointer(ep)), uintptr(unsafe.Pointer(tg)),
		uintptr(flags), 0,
		uintptr(unsafe.Pointer(&code)), uintptr(unsafe.Pointer(&msgs)))
	if code != nil {
		com.Adopt()
	}
	if msgs != nil {
		com.Adopt()
	}
	var text string
	if msgs != nil {
		text = string(blobBytes(msgs))
	}
	if herr := com.Check("D3DCompile", com.HRESULT(int32(uint32(r)))); herr != nil {
		com.Release(code)
		return nil, fmt.Errorf("%w: %v: %s", ErrCompile, herr, text)
	}
	if code == nil {
		return nil, errBlobEmpty
	}
	out := blobBytes(code)
	if out == nil {
		return nil, errBlobEmpty
	}
	return out, nil
}
