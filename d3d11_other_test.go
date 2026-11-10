// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build !windows

package d3d11

import (
	"errors"
	"testing"
)

func TestStub(t *testing.T) {
	d, err := CreateDevice(Options{Software: true})
	if d != nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(d, err)
	}
	(&Device{}).Close()
}
