// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build !windows

package d3dcompiler

import (
	"errors"
	"testing"
)

func TestStub(t *testing.T) {
	b, err := Compile(nil, "x.hlsl", "main", "cs_5_0", 0)
	if b != nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(b, err)
	}
}
