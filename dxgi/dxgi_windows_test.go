// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package dxgi

import (
	"errors"
	"testing"

	"github.com/go-mswin/d3d11/internal/com"
)

func TestFactoryAdapters(t *testing.T) {
	before := com.Live()
	f, err := CreateFactory1()
	if err != nil {
		t.Fatal(err)
	}
	descs, err := f.Adapters()
	if err != nil {
		t.Fatal(err)
	}
	if len(descs) == 0 {
		t.Fatal("no adapter: not even the Microsoft Basic Render Driver")
	}
	for i, d := range descs {
		t.Logf("adapter %d: %q vendor %#x software=%v vram=%d", i, d.Description, d.VendorID, d.Software(), d.DedicatedVideoMemory)
	}
	if _, err := f.EnumAdapter(MaxAdapters); !errors.Is(err, ErrNotFound) {
		t.Errorf("past the end: %v", err)
	}
	a, err := f.EnumAdapter(0)
	if err != nil {
		t.Fatal(err)
	}
	if a.Ptr() == nil {
		t.Error("Ptr")
	}
	a.Close()
	a.Close()
	f.Close()
	f.Close()
	if _, err := f.EnumAdapter(0); err == nil {
		t.Error("EnumAdapter on a closed factory")
	}
	if got := com.Live(); got != before {
		t.Fatalf("%d COM object(s) leaked", got-before)
	}
}
