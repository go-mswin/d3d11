// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

package dxgi

import "testing"

func TestAdapterDesc(t *testing.T) {
	if (AdapterDesc{}).Software() || !(AdapterDesc{Flags: AdapterFlagSoftware}).Software() {
		t.Fatal("Software flag")
	}
	if AdapterFlagSoftware != 2 { // DXGI_ADAPTER_FLAG_SOFTWARE
		t.Fatal("flag value")
	}
}
