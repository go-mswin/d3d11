// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

package com

import (
	"errors"
	"testing"
)

func TestHRESULTValues(t *testing.T) {
	// The constants are written as negative int32; these are the documented
	// unsigned codes.
	for _, c := range []struct {
		h    HRESULT
		want uint32
	}{
		{ENotImpl, 0x80004001}, {ENoInterface, 0x80004002}, {EFail, 0x80004005},
		{EOutOfMemory, 0x8007000E}, {EInvalidArg, 0x80070057},
		{DXGIInvalidCall, 0x887A0001}, {DXGINotFound, 0x887A0002},
		{DXGIUnsupported, 0x887A0004}, {DXGIDeviceRemoved, 0x887A0005},
		{DXGIDeviceHung, 0x887A0006}, {DXGIDeviceReset, 0x887A0007},
		{DXGIWasStillDrawing, 0x887A000A}, {DXGIDriverInternal, 0x887A0020},
		{D3D11TooManyStateObj, 0x887C0001}, {D3D11FileNotFound, 0x887C0002},
	} {
		if uint32(c.h) != c.want {
			t.Errorf("%#x != %#x", uint32(c.h), c.want)
		}
		if !c.h.Failed() {
			t.Errorf("%#x should fail", uint32(c.h))
		}
	}
	if SOK.Failed() || SFalse.Failed() {
		t.Error("S_OK and S_FALSE are successes")
	}
}

func TestCheckAndSentinels(t *testing.T) {
	if Check("op", SOK) != nil || Check("op", SFalse) != nil {
		t.Fatal("success must be nil")
	}
	cases := []struct {
		h    HRESULT
		want error
	}{
		{DXGIDeviceRemoved, ErrDeviceRemoved}, {DXGIDeviceHung, ErrDeviceRemoved},
		{DXGIDeviceReset, ErrDeviceRemoved}, {DXGIDriverInternal, ErrDeviceRemoved},
		{EOutOfMemory, ErrOutOfMemory},
		{EInvalidArg, ErrInvalidArg}, {DXGIInvalidCall, ErrInvalidArg},
		{DXGIUnsupported, ErrUnsupported}, {ENotImpl, ErrUnsupported}, {ENoInterface, ErrUnsupported},
		{DXGINotFound, ErrNotFound}, {DXGIWasStillDrawing, ErrNotReady},
	}
	all := []error{ErrDeviceRemoved, ErrOutOfMemory, ErrInvalidArg, ErrUnsupported, ErrNotFound, ErrNotReady}
	for _, c := range cases {
		err := Check("CreateBuffer", c.h)
		var e *Error
		if !errors.As(err, &e) || e.Code != c.h || e.Op != "CreateBuffer" {
			t.Fatalf("%#x: %v", uint32(c.h), err)
		}
		for _, s := range all {
			if got, want := errors.Is(err, s), s == c.want; got != want {
				t.Errorf("%#x Is(%v) = %v", uint32(c.h), s, got)
			}
		}
	}
	err := Check("Map", EFail)
	for _, s := range all {
		if errors.Is(err, s) {
			t.Errorf("E_FAIL must match no sentinel, matched %v", s)
		}
	}
	if got := err.Error(); got != "Map: HRESULT 0x80004005" {
		t.Fatal(got)
	}
}

func TestLive(t *testing.T) {
	base := Live()
	Adopt()
	Adopt()
	Drop()
	if Live() != base+1 {
		t.Fatal(Live())
	}
	Drop()
	if Live() != base {
		t.Fatal(Live())
	}
}
