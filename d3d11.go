// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package d3d11 is a pure-Go (no cgo) binding of the part of Direct3D 11 that a
// compute kernel needs: a device (hardware or WARP) at feature level 11_0 or
// better with its immediate context, structured, raw and constant buffers with
// SRV and UAV views, compute shaders from DXBC, Dispatch, and a staging buffer
// with Map/Unmap for read-back, with an event query to wait.
//
// COM is reached through syscall on vtable slots (see internal/com and
// slots.go). The package builds on every OS; off Windows [CreateDevice]
// returns [ErrUnsupportedPlatform]. Nothing here has been run on a GPU:
// the proof is a WARP device on a windows-latest runner.
//
// This file is the portable half: the sizes, limits and validation that are
// checked before any COM call.
package d3d11

import (
	"errors"
	"fmt"
	"math"

	"github.com/go-mswin/d3d11/dxgi"
	"github.com/go-mswin/d3d11/internal/com"
)

// HRESULT and Error are the COM result code and the error built from one.
type (
	HRESULT = com.HRESULT
	Error   = com.Error
)

// Sentinels, test with errors.Is. A device that is lost reports
// [ErrDeviceRemoved] from the call that noticed.
var (
	ErrDeviceRemoved = com.ErrDeviceRemoved
	ErrOutOfMemory   = com.ErrOutOfMemory
	ErrInvalidArg    = com.ErrInvalidArg
	ErrUnsupported   = com.ErrUnsupported
	// ErrUnsupportedPlatform is returned by CreateDevice off Windows.
	ErrUnsupportedPlatform = errors.New("d3d11: only available on Windows")
	// ErrTooLarge: a size above the documented limit, refused before creation.
	ErrTooLarge = errors.New("d3d11: size above the device limit")
	// ErrBadDesc: a description that breaks a documented rule.
	ErrBadDesc = errors.New("d3d11: invalid description")
	// ErrBadBytecode: bytes that are not a DXBC container.
	ErrBadBytecode = errors.New("d3d11: not DXBC bytecode")
	// ErrTimeout: Wait did not see the GPU finish in time.
	ErrTimeout = errors.New("d3d11: timed out waiting for the GPU")
	// ErrClosed: the object (or its device) was closed.
	ErrClosed = errors.New("d3d11: closed")
	// ErrMapped: the buffer is already mapped, or is not mapped.
	ErrMapped = errors.New("d3d11: buffer map state")
)

// Documented limits (D3D11 Functional Spec, constants table and §"Limits").
const (
	// The buffer-size expression: min(max(A, B * VRAM), C) MB, with
	// D3D11_REQ_RESOURCE_SIZE_IN_MEGABYTES_EXPRESSION_A_TERM = 128,
	// _B_TERM = 0.25f, _C_TERM = 2048.
	reqResourceSizeATermMB = 128
	reqResourceSizeCTermMB = 2048

	// MaxConstantBufferBytes is D3D11_REQ_CONSTANT_BUFFER_ELEMENT_COUNT (4096)
	// four-component 32-bit elements.
	MaxConstantBufferBytes = 4096 * 16
	// MaxStructureStride is D3D11_REQ_MULTI_ELEMENT_STRUCTURE_SIZE_IN_BYTES.
	MaxStructureStride = 2048
	// MaxDispatchGroups is D3D11_CS_DISPATCH_MAX_THREAD_GROUPS_PER_DIMENSION.
	MaxDispatchGroups = 65535
	// MaxSRVSlots, MaxUAVSlots, MaxConstantBufferSlots are the compute stage's
	// D3D11_COMMONSHADER_INPUT_RESOURCE_REGISTER_COUNT (128),
	// D3D11_PS_CS_UAV_REGISTER_COUNT (8) and
	// D3D11_COMMONSHADER_CONSTANT_BUFFER_API_SLOT_COUNT (14).
	MaxSRVSlots            = 128
	MaxUAVSlots            = 8
	MaxConstantBufferSlots = 14
)

// MaxBufferBytes is the largest buffer D3D11 is required to support on an
// adapter with the given dedicated video memory:
// min(max(128 MB, 0.25 * VRAM), 2048 MB). WARP and other software adapters
// report no dedicated memory and get 128 MB. A driver may accept more; this is
// the guaranteed figure, and a caller that has measured better can set
// [Options.MaxBufferBytes].
func MaxBufferBytes(dedicatedVideoMemory uint64) uint64 {
	const mb = 1 << 20
	m := max(uint64(reqResourceSizeATermMB)*mb, dedicatedVideoMemory/4)
	return min(m, uint64(reqResourceSizeCTermMB)*mb)
}

// BufferKind selects how a buffer is created.
type BufferKind int

// Buffer kinds.
const (
	// KindStructured: D3D11_RESOURCE_MISC_BUFFER_STRUCTURED, with Stride.
	KindStructured BufferKind = iota + 1
	// KindRaw: D3D11_RESOURCE_MISC_BUFFER_ALLOW_RAW_VIEWS (ByteAddressBuffer).
	KindRaw
	// KindConstant: a constant buffer, updated with Context.UpdateBuffer.
	KindConstant
	// KindStaging: a CPU-readable copy target for read-back.
	KindStaging
)

// BufferDesc describes a buffer. Size is in bytes.
type BufferDesc struct {
	Kind BufferKind
	Size uint64
	// Stride is the element size of a structured buffer, a multiple of 4.
	Stride uint32
	// SRV and UAV allow views of that kind (structured and raw only).
	SRV, UAV bool
	// Initial, when not nil, must be exactly Size bytes.
	Initial []byte
}

// ElementBytes is count * stride in bytes, computed in 64 bits so the product
// of two 32-bit values cannot wrap.
func ElementBytes(count, stride uint32) uint64 { return uint64(count) * uint64(stride) }

// validate checks a description against the documented rules and the device
// limit max, and returns the 32-bit ByteWidth. No COM call is made before it
// passes.
func (d BufferDesc) validate(maxBytes uint64) (uint32, error) {
	bad := func(format string, a ...any) (uint32, error) {
		return 0, fmt.Errorf("%w: %s", ErrBadDesc, fmt.Sprintf(format, a...))
	}
	if d.Size == 0 {
		return bad("zero size")
	}
	if d.Size > maxBytes {
		return 0, fmt.Errorf("%w: %d bytes, limit %d", ErrTooLarge, d.Size, maxBytes)
	}
	if d.Size > math.MaxUint32 { // ByteWidth is a UINT; only reachable with a raised MaxBufferBytes
		return 0, fmt.Errorf("%w: %d bytes does not fit ByteWidth", ErrTooLarge, d.Size)
	}
	if d.Initial != nil && uint64(len(d.Initial)) != d.Size {
		return bad("initial data is %d bytes, size is %d", len(d.Initial), d.Size)
	}
	if (d.Kind == KindStructured || d.Kind == KindRaw) && !d.SRV && !d.UAV {
		return bad("a structured or raw buffer needs SRV or UAV (it would have no bind flag)")
	}
	switch d.Kind {
	case KindStructured:
		if d.Stride == 0 || d.Stride%4 != 0 || d.Stride > MaxStructureStride {
			return bad("structured stride %d (a multiple of 4, at most %d)", d.Stride, MaxStructureStride)
		}
		if d.Size%uint64(d.Stride) != 0 {
			return bad("size %d is not a multiple of stride %d", d.Size, d.Stride)
		}
	case KindRaw:
		if d.Stride != 0 || d.Size%4 != 0 {
			return bad("raw buffer: stride must be 0 and size a multiple of 4")
		}
	case KindConstant:
		if d.Stride != 0 || d.SRV || d.UAV || d.Size%16 != 0 || d.Size > MaxConstantBufferBytes {
			return bad("constant buffer: size a multiple of 16 and at most %d, no stride or views", MaxConstantBufferBytes)
		}
	case KindStaging:
		if d.Stride != 0 || d.SRV || d.UAV {
			return bad("staging buffer takes no stride or views")
		}
	default:
		return bad("unknown kind %d", d.Kind)
	}
	return uint32(d.Size), nil
}

// checkDXBC checks that bytecode is plausibly a DXBC container: the "DXBC" magic
// and room for its 32-byte header (magic, 16-byte hash, version, total size,
// chunk count). D3D validates the rest.
func checkDXBC(b []byte) error {
	if len(b) < 32 || string(b[:4]) != "DXBC" {
		return ErrBadBytecode
	}
	return nil
}

// FeatureLevel is a D3D_FEATURE_LEVEL.
type FeatureLevel uint32

// Feature levels.
const (
	FeatureLevel10_0 FeatureLevel = 0xa000
	FeatureLevel10_1 FeatureLevel = 0xa100
	FeatureLevel11_0 FeatureLevel = 0xb000
	FeatureLevel11_1 FeatureLevel = 0xb100
)

func (l FeatureLevel) String() string {
	return fmt.Sprintf("%d_%d", (l>>12)&0xf, (l>>8)&0xf)
}

// Options configure CreateDevice. The zero value asks for a hardware device.
type Options struct {
	// Software creates a WARP device (D3D_DRIVER_TYPE_WARP): "the runtime
	// creates a WARP device and does not initialize a hardware device".
	Software bool
	// Adapter selects a hardware adapter from dxgi.Factory.EnumAdapter; it
	// excludes Software. nil means the default adapter.
	Adapter *dxgi.Adapter
	// MaxBufferBytes overrides the limit buffer sizes are checked against; 0
	// means [MaxBufferBytes] of the adapter's dedicated video memory.
	MaxBufferBytes uint64
}
