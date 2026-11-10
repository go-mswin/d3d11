// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package d3d11

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"github.com/go-mswin/d3d11/dxgi"
	"github.com/go-mswin/d3d11/internal/com"
	"golang.org/x/sys/windows"
)

var (
	modD3D11              = windows.NewLazySystemDLL("d3d11.dll")
	procD3D11CreateDevice = modD3D11.NewProc("D3D11CreateDevice")
)

// Enumerations and flags, from d3d11.h / dxgiformat.h.
const (
	driverTypeUnknown  = 0 // D3D_DRIVER_TYPE_UNKNOWN: required when an adapter is given
	driverTypeHardware = 1 // D3D_DRIVER_TYPE_HARDWARE
	driverTypeWARP     = 5 // D3D_DRIVER_TYPE_WARP
	sdkVersion         = 7 // D3D11_SDK_VERSION

	usageDefault = 0 // D3D11_USAGE_DEFAULT
	usageDynamic = 2 // D3D11_USAGE_DYNAMIC
	usageStaging = 3 // D3D11_USAGE_STAGING

	cpuAccessWrite  = 0x10000 // D3D11_CPU_ACCESS_WRITE
	mapWriteDiscard = 4       // D3D11_MAP_WRITE_DISCARD

	bindConstantBuffer  = 0x4  // D3D11_BIND_CONSTANT_BUFFER
	bindShaderResource  = 0x8  // D3D11_BIND_SHADER_RESOURCE
	bindUnorderedAccess = 0x80 // D3D11_BIND_UNORDERED_ACCESS

	cpuAccessRead = 0x20000 // D3D11_CPU_ACCESS_READ

	miscBufferAllowRawViews = 0x20 // D3D11_RESOURCE_MISC_BUFFER_ALLOW_RAW_VIEWS
	miscBufferStructured    = 0x40 // D3D11_RESOURCE_MISC_BUFFER_STRUCTURED

	mapRead = 1 // D3D11_MAP_READ

	formatUnknown    = 0  // DXGI_FORMAT_UNKNOWN
	formatR32Typeles = 39 // DXGI_FORMAT_R32_TYPELESS

	srvDimensionBuffer   = 1  // D3D11_SRV_DIMENSION_BUFFER
	srvDimensionBufferEx = 11 // D3D11_SRV_DIMENSION_BUFFEREX
	uavDimensionBuffer   = 1  // D3D11_UAV_DIMENSION_BUFFER

	bufferExSRVFlagRaw = 1 // D3D11_BUFFEREX_SRV_FLAG_RAW
	bufferUAVFlagRaw   = 1 // D3D11_BUFFER_UAV_FLAG_RAW

	queryEvent = 0 // D3D11_QUERY_EVENT (the first enumerator; 1 is OCCLUSION, whose GetData wants 8 bytes)
)

type bufferDesc struct {
	ByteWidth, Usage, BindFlags, CPUAccessFlags, MiscFlags, StructureByteStride uint32
}

type subresourceData struct {
	SysMem                        unsafe.Pointer
	SysMemPitch, SysMemSlicePitch uint32
}

// D3D11_SHADER_RESOURCE_VIEW_DESC is 24 bytes (its largest union member is the
// 16-byte Texture2DArray view); a buffer view uses the first 20.
type srvDesc struct {
	Format, ViewDimension, FirstElement, NumElements, Flags, _ uint32
}

// D3D11_UNORDERED_ACCESS_VIEW_DESC is 20 bytes.
type uavDesc struct {
	Format, ViewDimension, FirstElement, NumElements, Flags uint32
}

type mappedSubresource struct {
	Data                 unsafe.Pointer
	RowPitch, DepthPitch uint32
}

type queryDesc struct{ Query, MiscFlags uint32 }

// Device is an ID3D11Device and its immediate context. It is not safe for
// concurrent use: an immediate context belongs to one goroutine.
type Device struct {
	ptr     unsafe.Pointer
	ctx     *Context
	level   FeatureLevel
	adapter dxgi.AdapterDesc
	maxBuf  uint64
}

// CreateDevice creates a device at feature level 11_0 or better: hardware, or
// WARP with Options.Software. The result must be closed.
func CreateDevice(o Options) (*Device, error) {
	if o.Software && o.Adapter != nil {
		return nil, fmt.Errorf("%w: Software and Adapter are exclusive", ErrBadDesc)
	}
	if err := procD3D11CreateDevice.Find(); err != nil {
		return nil, fmt.Errorf("%w: D3D11CreateDevice: %v", ErrUnsupported, err)
	}
	driver := uintptr(driverTypeHardware)
	var adapter unsafe.Pointer
	switch {
	case o.Software:
		driver = driverTypeWARP
	case o.Adapter != nil:
		driver, adapter = driverTypeUnknown, o.Adapter.Ptr()
	}
	levels := [2]uint32{uint32(FeatureLevel11_1), uint32(FeatureLevel11_0)}
	var dev, ctx unsafe.Pointer
	var got uint32
	r, _, _ := procD3D11CreateDevice.Call(
		uintptr(adapter), driver, 0, 0,
		uintptr(unsafe.Pointer(&levels[0])), uintptr(len(levels)), sdkVersion,
		uintptr(unsafe.Pointer(&dev)), uintptr(unsafe.Pointer(&got)), uintptr(unsafe.Pointer(&ctx)))
	// Whatever came back is ours to release, success or not.
	if dev != nil {
		com.Adopt()
	}
	if ctx != nil {
		com.Adopt()
	}
	release := func() {
		com.Release(ctx)
		com.Release(dev)
	}
	if err := com.Check("D3D11CreateDevice", com.HRESULT(int32(uint32(r)))); err != nil {
		release()
		return nil, err
	}
	if dev == nil || ctx == nil {
		release()
		return nil, com.Check("D3D11CreateDevice", com.EFail)
	}
	if FeatureLevel(got) < FeatureLevel11_0 {
		release()
		return nil, fmt.Errorf("%w: feature level %s, need 11_0", ErrUnsupported, FeatureLevel(got))
	}
	d := &Device{ptr: dev, level: FeatureLevel(got)}
	d.ctx = &Context{dev: d, ptr: ctx}
	// The adapter's dedicated memory sets the buffer-size limit. If it cannot
	// be read, the guaranteed minimum (what WARP gets) applies.
	if desc, err := dxgi.DescribeDeviceAdapter(dev); err == nil {
		d.adapter = desc
	}
	d.maxBuf = MaxBufferBytes(d.adapter.DedicatedVideoMemory)
	if o.MaxBufferBytes != 0 {
		d.maxBuf = o.MaxBufferBytes
	}
	return d, nil
}

// Close releases the context and the device. It is idempotent. Resources made
// from the device stay valid until they are closed themselves (COM keeps the
// device alive for them).
func (d *Device) Close() {
	if d == nil || d.ptr == nil {
		return
	}
	com.Release(d.ctx.ptr)
	d.ctx.ptr = nil
	com.Release(d.ptr)
	d.ptr = nil
}

// FeatureLevel is the level the device was created at (at least 11_0).
func (d *Device) FeatureLevel() FeatureLevel { return d.level }

// Adapter describes the adapter, with Software() true for WARP.
func (d *Device) Adapter() dxgi.AdapterDesc { return d.adapter }

// MaxBufferBytes is the limit buffer sizes are checked against.
func (d *Device) MaxBufferBytes() uint64 { return d.maxBuf }

// Context is the immediate context.
func (d *Device) Context() *Context { return d.ctx }

// RemovedReason is ID3D11Device::GetDeviceRemovedReason: nil while the device
// is healthy, otherwise an error matching [ErrDeviceRemoved].
func (d *Device) RemovedReason() error {
	if d.ptr == nil {
		return ErrClosed
	}
	return com.Check("GetDeviceRemovedReason", com.Call0(d.ptr, devGetDeviceRemovedReason))
}

// fail turns a failing HRESULT into an error, replacing it by the removal
// reason when the device was lost (an op on a lost device often answers
// E_FAIL or E_INVALIDARG, and the cause is what a consumer must see).
func (d *Device) fail(op string, h HRESULT) error {
	err := com.Check(op, h)
	if err == nil || errors.Is(err, ErrDeviceRemoved) {
		return err
	}
	if r := com.Call0(d.ptr, devGetDeviceRemovedReason); r.Failed() {
		return com.Check(op, r)
	}
	return err
}

func (d *Device) open() error {
	if d == nil || d.ptr == nil {
		return ErrClosed
	}
	return nil
}

// Buffer is an ID3D11Buffer.
type Buffer struct {
	dev    *Device
	ptr    unsafe.Pointer
	desc   BufferDesc
	mapped bool
}

// CreateBuffer validates desc against the documented rules and the device's
// size limit, then creates the buffer. Nothing is created on a refusal.
func (d *Device) CreateBuffer(desc BufferDesc) (*Buffer, error) {
	if err := d.open(); err != nil {
		return nil, err
	}
	width, err := desc.validate(d.maxBuf)
	if err != nil {
		return nil, err
	}
	bd := bufferDesc{ByteWidth: width, Usage: usageDefault}
	switch desc.Kind {
	case KindStructured:
		bd.MiscFlags, bd.StructureByteStride = miscBufferStructured, desc.Stride
	case KindRaw:
		bd.MiscFlags = miscBufferAllowRawViews
	case KindConstant:
		bd.BindFlags = bindConstantBuffer
	case KindStaging:
		bd.Usage, bd.CPUAccessFlags = usageStaging, cpuAccessRead
	}
	if desc.Dynamic {
		bd.Usage, bd.CPUAccessFlags = usageDynamic, cpuAccessWrite
	}
	if desc.SRV {
		bd.BindFlags |= bindShaderResource
	}
	if desc.UAV {
		bd.BindFlags |= bindUnorderedAccess
	}
	var init *subresourceData
	if desc.Initial != nil {
		init = &subresourceData{SysMem: unsafe.Pointer(&desc.Initial[0])}
	}
	var p unsafe.Pointer
	h := com.Call3(d.ptr, devCreateBuffer, uintptr(unsafe.Pointer(&bd)), uintptr(unsafe.Pointer(init)), uintptr(unsafe.Pointer(&p)))
	if err := d.fail("CreateBuffer", h); err != nil {
		return nil, err
	}
	if p == nil {
		return nil, com.Check("CreateBuffer", com.EFail)
	}
	com.Adopt()
	desc.Initial = nil
	return &Buffer{dev: d, ptr: p, desc: desc}, nil
}

// Size is the buffer's size in bytes.
func (b *Buffer) Size() uint64 { return b.desc.Size }

// Close releases the buffer. It is idempotent.
func (b *Buffer) Close() {
	if b != nil && b.ptr != nil {
		com.Release(b.ptr)
		b.ptr = nil
	}
}

// SRV is an ID3D11ShaderResourceView.
type SRV struct{ ptr unsafe.Pointer }

// UAV is an ID3D11UnorderedAccessView.
type UAV struct{ ptr unsafe.Pointer }

// Close releases the view. It is idempotent.
func (v *SRV) Close() {
	if v != nil && v.ptr != nil {
		com.Release(v.ptr)
		v.ptr = nil
	}
}

// Close releases the view. It is idempotent.
func (v *UAV) Close() {
	if v != nil && v.ptr != nil {
		com.Release(v.ptr)
		v.ptr = nil
	}
}

func (b *Buffer) usable(op string, ok bool, what string) error {
	if b == nil || b.ptr == nil || b.dev.open() != nil {
		return ErrClosed
	}
	if !ok {
		return fmt.Errorf("%w: %s needs a buffer created with %s", ErrBadDesc, op, what)
	}
	return nil
}

// SRV creates a shader resource view of the whole buffer: a structured view for
// KindStructured, a raw (ByteAddressBuffer) view for KindRaw.
func (b *Buffer) SRV() (*SRV, error) {
	if err := b.usable("SRV", b.desc.SRV, "SRV: true"); err != nil {
		return nil, err
	}
	var vd srvDesc
	if b.desc.Kind == KindRaw {
		vd = srvDesc{Format: formatR32Typeles, ViewDimension: srvDimensionBufferEx,
			NumElements: uint32(b.desc.Size / 4), Flags: bufferExSRVFlagRaw}
	} else {
		vd = srvDesc{Format: formatUnknown, ViewDimension: srvDimensionBuffer,
			NumElements: uint32(b.desc.Size / uint64(b.desc.Stride))}
	}
	var p unsafe.Pointer
	h := com.Call3(b.dev.ptr, devCreateShaderResourceView, uintptr(b.ptr), uintptr(unsafe.Pointer(&vd)), uintptr(unsafe.Pointer(&p)))
	if err := b.dev.fail("CreateShaderResourceView", h); err != nil {
		return nil, err
	}
	if p == nil {
		return nil, com.Check("CreateShaderResourceView", com.EFail)
	}
	com.Adopt()
	return &SRV{ptr: p}, nil
}

// UAV creates an unordered access view of the whole buffer.
func (b *Buffer) UAV() (*UAV, error) {
	if err := b.usable("UAV", b.desc.UAV, "UAV: true"); err != nil {
		return nil, err
	}
	var vd uavDesc
	if b.desc.Kind == KindRaw {
		vd = uavDesc{Format: formatR32Typeles, ViewDimension: uavDimensionBuffer,
			NumElements: uint32(b.desc.Size / 4), Flags: bufferUAVFlagRaw}
	} else {
		vd = uavDesc{Format: formatUnknown, ViewDimension: uavDimensionBuffer,
			NumElements: uint32(b.desc.Size / uint64(b.desc.Stride))}
	}
	var p unsafe.Pointer
	h := com.Call3(b.dev.ptr, devCreateUnorderedAccessView, uintptr(b.ptr), uintptr(unsafe.Pointer(&vd)), uintptr(unsafe.Pointer(&p)))
	if err := b.dev.fail("CreateUnorderedAccessView", h); err != nil {
		return nil, err
	}
	if p == nil {
		return nil, com.Check("CreateUnorderedAccessView", com.EFail)
	}
	com.Adopt()
	return &UAV{ptr: p}, nil
}

// ComputeShader is an ID3D11ComputeShader.
type ComputeShader struct{ ptr unsafe.Pointer }

// Close releases the shader. It is idempotent.
func (s *ComputeShader) Close() {
	if s != nil && s.ptr != nil {
		com.Release(s.ptr)
		s.ptr = nil
	}
}

// CreateComputeShader creates a compute shader from DXBC bytes (cs_5_0).
func (d *Device) CreateComputeShader(dxbc []byte) (*ComputeShader, error) {
	if err := d.open(); err != nil {
		return nil, err
	}
	if err := checkDXBC(dxbc); err != nil {
		return nil, err
	}
	var p unsafe.Pointer
	h := com.Call4(d.ptr, devCreateComputeShader, uintptr(unsafe.Pointer(&dxbc[0])), uintptr(len(dxbc)), 0, uintptr(unsafe.Pointer(&p)))
	if err := d.fail("CreateComputeShader", h); err != nil {
		return nil, err
	}
	if p == nil {
		return nil, com.Check("CreateComputeShader", com.EFail)
	}
	com.Adopt()
	return &ComputeShader{ptr: p}, nil
}

// Context is the immediate context of a Device.
type Context struct {
	dev *Device
	ptr unsafe.Pointer
}

func (c *Context) open() error {
	if c == nil || c.ptr == nil {
		return ErrClosed
	}
	return nil
}

// SetShader binds a compute shader (nil unbinds).
func (c *Context) SetShader(s *ComputeShader) error {
	if err := c.open(); err != nil {
		return err
	}
	var p unsafe.Pointer
	if s != nil {
		p = s.ptr
	}
	com.Call3(c.ptr, ctxCSSetShader, uintptr(p), 0, 0)
	return nil
}

func slotRange(what string, start uint32, n, limit int) error {
	if n == 0 || uint64(start)+uint64(n) > uint64(limit) {
		return fmt.Errorf("%w: %d %s from slot %d (limit %d)", ErrBadDesc, n, what, start, limit)
	}
	return nil
}

// SetSRVs binds shader resource views from slot start (t#); nil unbinds one.
func (c *Context) SetSRVs(start uint32, views ...*SRV) error {
	if err := c.open(); err != nil {
		return err
	}
	if err := slotRange("SRVs", start, len(views), MaxSRVSlots); err != nil {
		return err
	}
	ptrs := make([]unsafe.Pointer, len(views))
	for i, v := range views {
		if v != nil {
			ptrs[i] = v.ptr
		}
	}
	com.Call3(c.ptr, ctxCSSetShaderResources, uintptr(start), uintptr(len(ptrs)), uintptr(unsafe.Pointer(&ptrs[0])))
	return nil
}

// SetUAVs binds unordered access views from slot start (u#); nil unbinds one.
func (c *Context) SetUAVs(start uint32, views ...*UAV) error {
	if err := c.open(); err != nil {
		return err
	}
	if err := slotRange("UAVs", start, len(views), MaxUAVSlots); err != nil {
		return err
	}
	ptrs := make([]unsafe.Pointer, len(views))
	for i, v := range views {
		if v != nil {
			ptrs[i] = v.ptr
		}
	}
	// pUAVInitialCounts is NULL: no append/consume buffer is used.
	com.Call4(c.ptr, ctxCSSetUnorderedAccessViews, uintptr(start), uintptr(len(ptrs)), uintptr(unsafe.Pointer(&ptrs[0])), 0)
	return nil
}

// SetConstantBuffers binds constant buffers from slot start (b#).
func (c *Context) SetConstantBuffers(start uint32, bufs ...*Buffer) error {
	if err := c.open(); err != nil {
		return err
	}
	if err := slotRange("constant buffers", start, len(bufs), MaxConstantBufferSlots); err != nil {
		return err
	}
	ptrs := make([]unsafe.Pointer, len(bufs))
	for i, b := range bufs {
		if b == nil {
			continue
		}
		if b.ptr == nil {
			return ErrClosed
		}
		if b.desc.Kind != KindConstant {
			return fmt.Errorf("%w: slot %d is not a constant buffer", ErrBadDesc, int(start)+i)
		}
		ptrs[i] = b.ptr
	}
	com.Call3(c.ptr, ctxCSSetConstantBuffers, uintptr(start), uintptr(len(ptrs)), uintptr(unsafe.Pointer(&ptrs[0])))
	return nil
}

// box is D3D11_BOX: bytes for a buffer, left to right.
type box struct{ Left, Top, Front, Right, Bottom, Back uint32 }

// UpdateBuffer writes data into a constant, structured or raw buffer from the
// CPU. The commands already queued keep seeing the old contents; those issued
// after see the new.
//
//   - A constant buffer is overwritten whole (len(data) == Size) with
//     UpdateSubresource and no box: "it is not possible to use this method to
//     partially update a shader-constant buffer".
//   - A Dynamic buffer (D3D11_USAGE_DYNAMIC, "a good choice for a resource that
//     will be updated by the CPU at least once per frame") is mapped with
//     WRITE_DISCARD, which is what the documentation recommends for it, and
//     data is copied to its start; len(data) <= Size, and the bytes past it are
//     undefined (the discard gave a fresh allocation).
//   - Any other structured or raw buffer is DEFAULT usage, which a shader may
//     write: UpdateSubresource copies data to its start (a box of len(data)
//     bytes when that is less than Size, the rest keeps its contents). This is
//     how the target of a compute pass is given the CPU's ground.
//
// UpdateSubresource copies data before it returns, so the caller may reuse data
// at once.
func (c *Context) UpdateBuffer(b *Buffer, data []byte) error {
	if err := c.open(); err != nil {
		return err
	}
	ok := b != nil && (b.desc.Kind == KindConstant || b.desc.Kind == KindStructured || b.desc.Kind == KindRaw)
	if err := b.usable("UpdateBuffer", ok, "KindConstant, KindStructured or KindRaw"); err != nil {
		return err
	}
	n := uint64(len(data))
	switch {
	case b.desc.Kind == KindConstant:
		if n != b.desc.Size {
			return fmt.Errorf("%w: %d bytes for a %d-byte constant buffer", ErrBadDesc, n, b.desc.Size)
		}
	case n == 0 || n > b.desc.Size:
		return fmt.Errorf("%w: %d bytes for a %d-byte buffer", ErrBadDesc, n, b.desc.Size)
	}
	if b.desc.Dynamic {
		var ms mappedSubresource
		h := com.Call5(c.ptr, ctxMap, uintptr(b.ptr), 0, mapWriteDiscard, 0, uintptr(unsafe.Pointer(&ms)))
		if err := c.dev.fail("Map", h); err != nil {
			return err
		}
		if ms.Data == nil {
			return com.Check("Map", com.EFail)
		}
		copy(unsafe.Slice((*byte)(ms.Data), int(b.desc.Size)), data)
		com.Call2(c.ptr, ctxUnmap, uintptr(b.ptr), 0)
		return nil
	}
	var dst *box
	if n < b.desc.Size {
		dst = &box{Right: uint32(n), Bottom: 1, Back: 1}
	}
	com.Call6(c.ptr, ctxUpdateSubresource, uintptr(b.ptr), 0, uintptr(unsafe.Pointer(dst)), uintptr(unsafe.Pointer(&data[0])), 0, 0)
	return nil
}

// Flush sends the queued commands to the GPU without waiting for them
// (ID3D11DeviceContext::Flush), so that work submitted now runs while the CPU
// does something else. Wait and Fence.Wait flush too.
func (c *Context) Flush() error {
	if err := c.open(); err != nil {
		return err
	}
	com.Call0(c.ptr, ctxFlush)
	return nil
}

// Fence is an event query marking a point in the context's command stream:
// done once the GPU has finished every command issued before Signal.
type Fence struct {
	ctx *Context
	q   unsafe.Pointer
}

// Signal ends an event query after the commands issued so far. Close the Fence.
// One per submitted frame lets each frame be waited for on its own.
func (c *Context) Signal() (*Fence, error) {
	if err := c.open(); err != nil {
		return nil, err
	}
	d := c.dev
	qd := queryDesc{Query: queryEvent}
	var q unsafe.Pointer
	if err := d.fail("CreateQuery", com.Call2(d.ptr, devCreateQuery, uintptr(unsafe.Pointer(&qd)), uintptr(unsafe.Pointer(&q)))); err != nil {
		return nil, err
	}
	if q == nil {
		return nil, com.Check("CreateQuery", com.EFail)
	}
	com.Adopt()
	com.Call1(c.ptr, ctxEnd, uintptr(q))
	return &Fence{ctx: c, q: q}, nil
}

// Done reports whether the GPU has passed the fence. It flushes, since a
// command that was never sent would never finish.
func (f *Fence) Done() (bool, error) {
	if f == nil || f.q == nil {
		return false, ErrClosed
	}
	c := f.ctx
	if err := c.open(); err != nil {
		return false, err
	}
	var done uint32 // BOOL
	h := com.Call4(c.ptr, ctxGetData, uintptr(f.q), uintptr(unsafe.Pointer(&done)), 4, 0)
	if err := c.dev.fail("GetData", h); err != nil {
		return false, err
	}
	return h == com.SOK && done != 0, nil // S_FALSE: not finished yet
}

// Wait blocks until the GPU has passed the fence, or ErrTimeout; the removal
// error if the device was lost meanwhile.
func (f *Fence) Wait(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for spin := 0; ; spin++ {
		done, err := f.Done()
		if err != nil || done {
			return err
		}
		if !time.Now().Before(deadline) {
			if err := f.ctx.dev.RemovedReason(); err != nil {
				return err
			}
			return ErrTimeout
		}
		if spin < 1000 {
			runtime.Gosched()
		} else {
			time.Sleep(50 * time.Microsecond)
		}
	}
}

// Close releases the query. It is idempotent.
func (f *Fence) Close() {
	if f != nil && f.q != nil {
		com.Release(f.q)
		f.q = nil
	}
}

// Dispatch runs the bound compute shader on x*y*z thread groups.
func (c *Context) Dispatch(x, y, z uint32) error {
	if err := c.open(); err != nil {
		return err
	}
	if x > MaxDispatchGroups || y > MaxDispatchGroups || z > MaxDispatchGroups {
		return fmt.Errorf("%w: dispatch %dx%dx%d, at most %d groups per dimension", ErrBadDesc, x, y, z, MaxDispatchGroups)
	}
	com.Call3(c.ptr, ctxDispatch, uintptr(x), uintptr(y), uintptr(z))
	return nil
}

// CopyBuffer copies src to dst (CopyResource); both must be the same size.
func (c *Context) CopyBuffer(dst, src *Buffer) error {
	if err := c.open(); err != nil {
		return err
	}
	if dst == nil || src == nil || dst.ptr == nil || src.ptr == nil {
		return ErrClosed
	}
	if dst.desc.Size != src.desc.Size {
		return fmt.Errorf("%w: copy of %d bytes into %d", ErrBadDesc, src.desc.Size, dst.desc.Size)
	}
	com.Call2(c.ptr, ctxCopyResource, uintptr(dst.ptr), uintptr(src.ptr))
	return nil
}

// Wait blocks until every command issued so far has finished on the GPU: it
// ends a D3D11_QUERY_EVENT and polls GetData (which flushes). It gives up with
// [ErrTimeout] after timeout, and returns the removal error if the device was
// lost meanwhile. The query is released on every path.
func (c *Context) Wait(timeout time.Duration) error {
	f, err := c.Signal()
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Wait(timeout)
}

// MapRead maps a staging buffer for reading. The returned slice aliases the
// driver's memory: it is valid only until [Context.Unmap], and must not be
// kept. Bounds are the buffer's size.
func (c *Context) MapRead(b *Buffer) ([]byte, error) {
	if err := c.open(); err != nil {
		return nil, err
	}
	if err := b.usable("MapRead", b != nil && b.desc.Kind == KindStaging, "KindStaging"); err != nil {
		return nil, err
	}
	if b.mapped {
		return nil, fmt.Errorf("%w: already mapped", ErrMapped)
	}
	var ms mappedSubresource
	h := com.Call5(c.ptr, ctxMap, uintptr(b.ptr), 0, mapRead, 0, uintptr(unsafe.Pointer(&ms)))
	if err := c.dev.fail("Map", h); err != nil {
		return nil, err
	}
	if ms.Data == nil {
		return nil, com.Check("Map", com.EFail)
	}
	b.mapped = true
	return unsafe.Slice((*byte)(ms.Data), int(b.desc.Size)), nil
}

// Unmap ends a MapRead. The slice it returned must not be used afterwards.
func (c *Context) Unmap(b *Buffer) error {
	if err := c.open(); err != nil {
		return err
	}
	if b == nil || b.ptr == nil {
		return ErrClosed
	}
	if !b.mapped {
		return fmt.Errorf("%w: not mapped", ErrMapped)
	}
	com.Call2(c.ptr, ctxUnmap, uintptr(b.ptr), 0)
	b.mapped = false
	return nil
}

// Readback copies any buffer to a temporary staging buffer, waits for the GPU
// and returns the bytes in a Go slice the caller owns. The staging buffer is
// released on every path.
func (c *Context) Readback(src *Buffer, timeout time.Duration) ([]byte, error) {
	if err := c.open(); err != nil {
		return nil, err
	}
	if src == nil || src.ptr == nil {
		return nil, ErrClosed
	}
	st, err := c.dev.CreateBuffer(BufferDesc{Kind: KindStaging, Size: src.desc.Size})
	if err != nil {
		return nil, err
	}
	defer st.Close()
	if err := c.CopyBuffer(st, src); err != nil {
		return nil, err
	}
	if err := c.Wait(timeout); err != nil {
		return nil, err
	}
	m, err := c.MapRead(st)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(m))
	copy(out, m)
	if err := c.Unmap(st); err != nil {
		return nil, err
	}
	return out, nil
}
