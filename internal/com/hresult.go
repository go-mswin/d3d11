// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package com is the COM plumbing shared by d3d11, dxgi and d3dcompiler: the
// HRESULT type and its mapping to Go errors (portable, tested everywhere), and
// the vtable call machinery (Windows only).
package com

import (
	"errors"
	"fmt"
	"sync/atomic"
)

// HRESULT is a COM result code.
type HRESULT int32

// Result codes this module maps. The values are the documented ones (winerror.h
// and dxgitype.h / d3d11.h).
const (
	SOK                  HRESULT = 0
	SFalse               HRESULT = 1
	ENotImpl             HRESULT = -0x7FFFBFFF // 0x80004001
	ENoInterface         HRESULT = -0x7FFFBFFE // 0x80004002
	EFail                HRESULT = -0x7FFFBFFB // 0x80004005
	EOutOfMemory         HRESULT = -0x7FF8FFF2 // 0x8007000E
	EInvalidArg          HRESULT = -0x7FF8FFA9 // 0x80070057
	DXGIInvalidCall      HRESULT = -0x7785FFFF // 0x887A0001
	DXGINotFound         HRESULT = -0x7785FFFE // 0x887A0002
	DXGIUnsupported      HRESULT = -0x7785FFFC // 0x887A0004
	DXGIDeviceRemoved    HRESULT = -0x7785FFFB // 0x887A0005
	DXGIDeviceHung       HRESULT = -0x7785FFFA // 0x887A0006
	DXGIDeviceReset      HRESULT = -0x7785FFF9 // 0x887A0007
	DXGIWasStillDrawing  HRESULT = -0x7785FFF6 // 0x887A000A
	DXGIDriverInternal   HRESULT = -0x7785FFE0 // 0x887A0020
	D3D11TooManyStateObj HRESULT = -0x7783FFFF // 0x887C0001
	D3D11FileNotFound    HRESULT = -0x7783FFFE // 0x887C0002
)

// Failed reports whether h is a failure code (severity bit set).
func (h HRESULT) Failed() bool { return h < 0 }

// Sentinels. Test with errors.Is.
var (
	// ErrDeviceRemoved covers DXGI_ERROR_DEVICE_REMOVED, _HUNG, _RESET and
	// _DRIVER_INTERNAL_ERROR: the device is gone and every object made from it
	// is unusable. A consumer drops to its CPU path.
	ErrDeviceRemoved = errors.New("d3d11: device removed")
	// ErrOutOfMemory is E_OUTOFMEMORY, which CreateBuffer also answers for a
	// size above the device's resource-size limit.
	ErrOutOfMemory = errors.New("d3d11: out of memory")
	// ErrInvalidArg is E_INVALIDARG and DXGI_ERROR_INVALID_CALL.
	ErrInvalidArg = errors.New("d3d11: invalid argument")
	// ErrUnsupported is DXGI_ERROR_UNSUPPORTED, E_NOTIMPL and E_NOINTERFACE.
	ErrUnsupported = errors.New("d3d11: unsupported")
	// ErrNotFound is DXGI_ERROR_NOT_FOUND (the end of an enumeration).
	ErrNotFound = errors.New("d3d11: not found")
	// ErrNotReady is DXGI_ERROR_WAS_STILL_DRAWING.
	ErrNotReady = errors.New("d3d11: still drawing")
)

// Error is a failing HRESULT with the call that produced it.
type Error struct {
	Op   string
	Code HRESULT
}

func (e *Error) Error() string { return fmt.Sprintf("%s: HRESULT 0x%08X", e.Op, uint32(e.Code)) }

// Is maps the code to the sentinels above.
func (e *Error) Is(target error) bool {
	switch target {
	case ErrDeviceRemoved:
		switch e.Code {
		case DXGIDeviceRemoved, DXGIDeviceHung, DXGIDeviceReset, DXGIDriverInternal:
			return true
		}
	case ErrOutOfMemory:
		return e.Code == EOutOfMemory
	case ErrInvalidArg:
		return e.Code == EInvalidArg || e.Code == DXGIInvalidCall
	case ErrUnsupported:
		return e.Code == DXGIUnsupported || e.Code == ENotImpl || e.Code == ENoInterface
	case ErrNotFound:
		return e.Code == DXGINotFound
	case ErrNotReady:
		return e.Code == DXGIWasStillDrawing
	}
	return false
}

// Check turns a failing HRESULT into an *Error, and a success into nil.
func Check(op string, h HRESULT) error {
	if !h.Failed() {
		return nil
	}
	return &Error{Op: op, Code: h}
}

// live counts COM references this module owns: Adopt on creation, Drop on
// Release. Tests assert it returns to its starting value, which is how "every
// COM object is released on every error path" is checked rather than claimed.
var live atomic.Int64

// Adopt records one owned object.
func Adopt() { live.Add(1) }

// Drop records the release of one owned object.
func Drop() { live.Add(-1) }

// Live is the number of owned objects not yet released.
func Live() int64 { return live.Load() }
