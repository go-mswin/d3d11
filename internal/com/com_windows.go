// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package com

import (
	"syscall"
	"unsafe"
)

// GUID is a Windows GUID in its in-memory layout.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// IUnknown slots, shared by every interface.
const (
	SlotQueryInterface = 0
	SlotAddRef         = 1
	SlotRelease        = 2
)

// Slot returns the nth entry of a COM object's vtable. A COM interface pointer
// points at a structure whose first word points at an array of function
// pointers in declaration order, base interfaces first (the technique and its
// risk are described in go-mswin/screencapture dxgi_windows.go: a wrong slot
// calls a different method and returns a plausible HRESULT).
func Slot(obj unsafe.Pointer, n int) uintptr {
	return (*(**[256]uintptr)(obj))[n]
}

// Call0..Call6 invoke vtable slot n of obj with obj as `this` and up to six
// arguments, and return the HRESULT. They are fixed-arity, not variadic,
// because //go:uintptrescapes only covers plain uintptr parameters: it makes
// the compiler keep whatever `uintptr(unsafe.Pointer(&x))` points at on the
// heap and alive for the call. Without it, x could sit on the goroutine stack,
// which the runtime may move (stack growth) between building the uintptr and
// the call, leaving the callee a stale address.

//go:uintptrescapes
func Call0(obj unsafe.Pointer, n int) HRESULT { return call(obj, n, 0, 0, 0, 0, 0, 0) }

//go:uintptrescapes
func Call1(obj unsafe.Pointer, n int, a1 uintptr) HRESULT { return call(obj, n, a1, 0, 0, 0, 0, 0) }

//go:uintptrescapes
func Call2(obj unsafe.Pointer, n int, a1, a2 uintptr) HRESULT {
	return call(obj, n, a1, a2, 0, 0, 0, 0)
}

//go:uintptrescapes
func Call3(obj unsafe.Pointer, n int, a1, a2, a3 uintptr) HRESULT {
	return call(obj, n, a1, a2, a3, 0, 0, 0)
}

//go:uintptrescapes
func Call4(obj unsafe.Pointer, n int, a1, a2, a3, a4 uintptr) HRESULT {
	return call(obj, n, a1, a2, a3, a4, 0, 0)
}

//go:uintptrescapes
func Call5(obj unsafe.Pointer, n int, a1, a2, a3, a4, a5 uintptr) HRESULT {
	return call(obj, n, a1, a2, a3, a4, a5, 0)
}

//go:uintptrescapes
func Call6(obj unsafe.Pointer, n int, a1, a2, a3, a4, a5, a6 uintptr) HRESULT {
	return call(obj, n, a1, a2, a3, a4, a5, a6)
}

// call always passes seven words; a callee that takes fewer ignores the rest
// (the Windows calling conventions leave cleanup to the caller's frame, which
// syscall restores).
func call(obj unsafe.Pointer, n int, a1, a2, a3, a4, a5, a6 uintptr) HRESULT {
	r, _, _ := syscall.SyscallN(Slot(obj, n), uintptr(obj), a1, a2, a3, a4, a5, a6)
	return HRESULT(int32(uint32(r)))
}

// Release drops one reference on an object this module owns. nil is ignored.
func Release(obj unsafe.Pointer) {
	if obj == nil {
		return
	}
	syscall.SyscallN(Slot(obj, SlotRelease), uintptr(obj))
	Drop()
}

// QueryInterface asks obj for another interface. The result is owned.
func QueryInterface(obj unsafe.Pointer, id *GUID, op string) (unsafe.Pointer, error) {
	var out unsafe.Pointer
	h := Call2(obj, SlotQueryInterface, uintptr(unsafe.Pointer(id)), uintptr(unsafe.Pointer(&out)))
	if err := Check(op, h); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, Check(op, ENoInterface)
	}
	Adopt()
	return out, nil
}

// CallPtr calls a slot that returns a pointer instead of an HRESULT (for
// example ID3DBlob::GetBufferPointer). The result is read back through the
// variable's own storage, so no uintptr is ever converted to a pointer.
func CallPtr(obj unsafe.Pointer, n int) unsafe.Pointer {
	r, _, _ := syscall.SyscallN(Slot(obj, n), uintptr(obj))
	return *(*unsafe.Pointer)(unsafe.Pointer(&r))
}

// CallSize calls a slot that returns a SIZE_T (for example
// ID3DBlob::GetBufferSize).
func CallSize(obj unsafe.Pointer, n int) uintptr {
	r, _, _ := syscall.SyscallN(Slot(obj, n), uintptr(obj))
	return r
}
