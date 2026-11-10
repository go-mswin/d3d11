# d3d11

A pure-Go (no cgo) binding of the part of Direct3D 11 that a **compute kernel**
needs, with package `dxgi` alongside. COM is reached through
`syscall.SyscallN` on vtable slots, in the style of
[go-mswin/screencapture](https://github.com/go-mswin/screencapture).

```
go get github.com/go-mswin/d3d11
```

Windows only; on every other OS the packages build and every constructor
returns `ErrUnsupportedPlatform`, so a consumer cross-compiles.

**What has been proven, and where.** Nothing here has run on a GPU: no Windows
GPU machine exists for this project. The proof is the `windows` CI job on
`windows-latest`, where Direct3D runs on **WARP** (`D3D_DRIVER_TYPE_WARP`), the
software rasterizer that ships with Windows. It proves the bindings (slots,
structure layouts, call order) and the documented semantics under WARP's
arithmetic. It does not prove speed or the behaviour of a hardware driver.
Treat the hardware path as opt-in until someone runs the suite on a GPU.

## Surface

```go
dev, err := d3d11.CreateDevice(d3d11.Options{Software: true}) // WARP; zero value = hardware
defer dev.Close()                                              // feature level >= 11_0 or an error

dst, _ := dev.CreateBuffer(d3d11.BufferDesc{Kind: d3d11.KindStructured, Size: 4096, Stride: 4, UAV: true})
uav, _ := dst.UAV()
cb, _  := dev.CreateBuffer(d3d11.BufferDesc{Kind: d3d11.KindConstant, Size: 16, Initial: params})
sh, _  := dev.CreateComputeShader(dxbc)

ctx := dev.Context()
ctx.SetShader(sh); ctx.SetConstantBuffers(0, cb); ctx.SetUAVs(0, uav)
ctx.Dispatch(16, 1, 1)
out, err := ctx.Readback(dst, 5*time.Second) // staging copy + event query + Map/Unmap
```

| Area | API |
|---|---|
| Device | `CreateDevice(Options)`, `Device.FeatureLevel/Adapter/MaxBufferBytes/RemovedReason/Close` |
| Buffers | `CreateBuffer(BufferDesc)`: `KindStructured`, `KindRaw`, `KindConstant`, `KindStaging` |
| Views | `Buffer.SRV()`, `Buffer.UAV()` (structured, or raw `ByteAddressBuffer` views) |
| Compute | `CreateComputeShader(dxbc)`, `Context.SetShader/SetSRVs/SetUAVs/SetConstantBuffers/UpdateBuffer/Dispatch` |
| Read-back | `CopyBuffer`, `Wait(timeout)` (`D3D11_QUERY_EVENT`), `MapRead`/`Unmap`, `Readback` |
| Errors | `*Error{Op, Code}`; sentinels `ErrDeviceRemoved`, `ErrOutOfMemory`, `ErrInvalidArg`, `ErrUnsupported`, `ErrTooLarge`, `ErrBadDesc`, `ErrBadBytecode`, `ErrTimeout`, `ErrClosed` |
| `dxgi` | `CreateFactory1`, `Factory.EnumAdapter/Adapters`, `Adapter.Desc/Ptr`, `DescribeDeviceAdapter` |
| `d3dcompiler` | `Compile(hlsl, name, entry, target, flags)`: `D3DCompile` through syscall |

`DXGI_ERROR_DEVICE_REMOVED`, `_HUNG`, `_RESET` and `_DRIVER_INTERNAL_ERROR` all
match `errors.Is(err, ErrDeviceRemoved)`. When a call fails, the device is asked
`GetDeviceRemovedReason` and, if it was lost, that is the error returned, because
a call on a lost device often answers `E_FAIL` or `E_INVALIDARG`.

The swap chain (`IDXGIFactory2::CreateSwapChainForHwnd`, `GetBuffer`,
`Present1`, `ResizeBuffers`) is milestone D4 and is not bound yet. `dxgi.Factory`
holds the `IDXGIFactory1` pointer it will be queried from.

## Safety

- **Slots.** Every vtable slot is a position in a full list of the interface's
  methods in `d3d11.h` order (`slots.go`), pinned by anchors in a test that runs
  on every OS. A wrong slot calls a different method, so the WARP dispatch is
  the end-to-end check.
- **Sizes are checked before creation.** `BufferDesc` is validated against the
  documented rules (stride, alignment, constant buffer 65536 bytes, at least one
  view) and the device limit; a refusal makes no COM call. Products are 64-bit
  and every size must fit the 32-bit `ByteWidth`.
- **Release.** Every COM object this module owns is counted
  (`internal/com.Live`); each WARP test asserts the count returns to where it
  started, including the error paths.
- **Pointers.** `Call0..Call6` carry `//go:uintptrescapes`, so a pointer turned
  into a `uintptr` for a call cannot sit on a stack the runtime may move.
  `Map` hands back a slice aliasing driver memory, valid until `Unmap`.
- **Untrusted input.** There is no parser or decoder in this module. The only
  bytes it accepts are DXBC from the caller, checked for the `DXBC` magic and a
  header's room, then validated by Direct3D. A shader is not a security
  boundary here.

## The two open items of gpu2d's `docs/NATIVE.md`, answered

Source: Direct3D 11.3 Functional Specification,
<https://microsoft.github.io/DirectX-Specs/d3d/archive/D3D11_3_FunctionalSpec.htm>
(Microsoft; opened and read for this module).

### 1. Maximum buffer size

The maximum size of a buffer, and of any resource, is
`min(max(128, 0.25 * (amount of dedicated VRAM)), 2048)` MB. The three constants
are `D3D11_REQ_RESOURCE_SIZE_IN_MEGABYTES_EXPRESSION_A_TERM` = 128,
`_B_TERM` = 0.25f, `_C_TERM` = 2048 (the constants table, and the "Resource Size
in MB for any of the above Resources" row of the resource limits table). A raw
buffer view can address all of it: "addressing is at 32-bit granularity and the
entire contents of the Raw Buffer view is available, at most 2048 MB".
Two consequences:

- The **guaranteed** floor is **128 MiB**, which is what WARP and any adapter
  with under 512 MiB of dedicated memory get (WARP reports none). 2048 MiB is
  reached at 8 GiB of VRAM. The limit always fits a 32-bit `ByteWidth`.
- It is a requirement, not a ceiling: a driver may accept more. `MaxBufferBytes`
  returns the guaranteed figure for the adapter's `DedicatedVideoMemory`;
  `Options.MaxBufferBytes` overrides it for a caller who has measured better.
  Constant buffers are separately limited to 4096 four-component elements (65536
  bytes), `D3D11_REQ_CONSTANT_BUFFER_ELEMENT_COUNT`.

For gpu2d: `MaxData` is 2^28 bytes (256 MiB) and `MaxBinned` entries take 2^28
bytes, so a frame can exceed the 128 MiB floor and must be refused with
`ErrTooLarge` (as the Vulkan path does), unless the device's limit is higher.

### 2. Out-of-bounds read of a structured buffer

Specification, `ld_structured` (instruction reference 22.4.12): "Out of bounds
addressing on u#/t# of any given 32-bit component returns 0 for that component,
except: If srcByteOffset (plus swizzle) is what causes out of bounds access to
u#/t#, the returned value for all component(s) is undefined." Bounds are
relative to the view ("Out of range behaviors are also always with respect to
the view properties where views are used"). `ld_raw` (22.4.10) returns 0 per
component, with no exception. `D3D11_NONSAMPLE_FETCH_OUT_OF_RANGE_ACCESS_RESULT`
is 0.

So: an out-of-range **struct index** reads **0**; an out-of-range **byte offset
inside a struct** is **undefined**, and a kernel must never rely on it. Writes
(`store_structured`, `store_raw`): an out-of-bounds struct index writes nothing
("nothing is written to the out of bounds memory"), but "out of bounds offsets
within individual structs cause the entire contents of the resource to be
undefined" (the device-memory write rules).

`TestWARPStructuredOutOfBoundsReadsZero` checks the first case on WARP.
This is a software implementation's answer: a hardware driver is bound by the
same specification, but its behaviour is unmeasured here. For gpu2d, every index
comes from the CPU, so the guarantee is a second line of defence, as Vulkan's
`robustBufferAccess` is.

## DXBC: compiled on the Windows CI lane and committed

HLSL is compiled to DXBC with **`D3DCompile` from the system
`d3dcompiler_47.dll`, through syscall** (package `d3dcompiler`), not with
`fxc.exe`. Why: it keeps the whole path in Go, with the same calling mechanism as
the rest of the module; it needs nothing installed (the DLL ships in System32 on
every Windows 10 and later, whereas `fxc.exe` lives in a versioned Windows SDK
directory on the runner); and the drift test can call it in-process.

`testdata/kernels.hlsl` is hand-written (the HLSL emitter is a later milestone);
its entry points `fill`, `rawadd` and `oob` are compiled for `cs_5_0` and the
bytes are committed as `testdata/<entry>.dxbc`. `TestDXBCNotDrifted` recompiles
on the Windows lane and compares. To regenerate, run the test on Windows with
`D3D11_DXBC_OUT=<dir>` (the CI job uploads that directory as the `dxbc`
artifact), copy the files over `testdata/` and commit. DXBC embeds a hash and the
compiler's output can change with `d3dcompiler_47.dll`; if the runner image
updates it, the drift test goes red and the files are regenerated: that is the
test doing its job.

## Tests and CI

- `portable` (Ubuntu x86-64 and arm64, macOS): gofmt, vet, `-race`, **100 %
  coverage** of everything that builds there (limits, validation, HRESULT
  mapping, slots, stubs).
- `windows` (windows-latest, WARP): vet, `-race`, the dispatch proofs, the break
  check, the leak counter, the drift test.
- `linux-386`, `cross` (six 64-bit Linux architectures, and Windows
  amd64/arm64/386, vet with tests), `govulncheck` (Linux and Windows builds).

The proof of the D1 milestone: a WARP device dispatches the fill shader and the
staging read-back has the expected bytes (`TestWARPFillDispatch`); the same
dispatch with the UAV bound at the wrong slot must fail the comparison and leaves
the buffer zero (`TestWrongUAVBindingFails`).

## Licence

BSD 3-Clause.
