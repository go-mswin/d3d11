// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

package d3d11

import "testing"

func TestSlots(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want int
	}{
		// First and last of each list.
		{"devCreateBuffer", devCreateBuffer, 3},
		{"devGetExceptionMode", devGetExceptionMode, 42},
		{"ctxVSSetConstantBuffers", ctxVSSetConstantBuffers, 7},
		{"ctxFinishCommandList", ctxFinishCommandList, 114},
		// The slots go-mswin/screencapture/dxgi_windows.go already uses.
		{"devCreateTexture2D", devCreateTexture2D, 5},
		{"ctxMap", ctxMap, 14},
		{"ctxUnmap", ctxUnmap, 15},
		{"ctxCopyResource", ctxCopyResource, 47},
		// The ones this package calls.
		{"devCreateShaderResourceView", devCreateShaderResourceView, 7},
		{"devCreateUnorderedAccessView", devCreateUnorderedAccessView, 8},
		{"devCreateComputeShader", devCreateComputeShader, 18},
		{"devCreateQuery", devCreateQuery, 24},
		{"devGetDeviceRemovedReason", devGetDeviceRemovedReason, 39},
		{"ctxBegin", ctxBegin, 27},
		{"ctxEnd", ctxEnd, 28},
		{"ctxGetData", ctxGetData, 29},
		{"ctxDispatch", ctxDispatch, 41},
		{"ctxUpdateSubresource", ctxUpdateSubresource, 48},
		{"ctxCSSetShaderResources", ctxCSSetShaderResources, 67},
		{"ctxCSSetUnorderedAccessViews", ctxCSSetUnorderedAccessViews, 68},
		{"ctxCSSetShader", ctxCSSetShader, 69},
		{"ctxCSSetConstantBuffers", ctxCSSetConstantBuffers, 71},
		{"ctxFlush", ctxFlush, 111},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}
