// Test kernels for go-mswin/d3d11 (milestone D1). Hand-written: the hlsl
// emitter of go-gfx/wgsl is D2. Compiled with D3DCompile, cs_5_0, one blob per
// entry point, committed as testdata/<entry>.dxbc.

cbuffer Params : register(b0)
{
    uint count; // elements to process
    uint value; // fill: base value; rawadd: addend; oob: index offset
    uint2 pad;
};

StructuredBuffer<uint>   src    : register(t0);
ByteAddressBuffer        rawSrc : register(t1);
RWStructuredBuffer<uint> dst    : register(u0);
RWByteAddressBuffer      rawDst : register(u1);

// dst[i] = value + i for i < count; nothing else is written.
[numthreads(64, 1, 1)]
void fill(uint3 id : SV_DispatchThreadID)
{
    if (id.x < count)
        dst[id.x] = value + id.x;
}

// rawDst[i] = rawSrc[i] + value, through byte addresses.
[numthreads(64, 1, 1)]
void rawadd(uint3 id : SV_DispatchThreadID)
{
    if (id.x < count)
        rawDst.Store(id.x * 4, rawSrc.Load(id.x * 4) + value);
}

// dst[i] = src[i + value]: with value > 0 the last reads are out of bounds.
[numthreads(64, 1, 1)]
void oob(uint3 id : SV_DispatchThreadID)
{
    if (id.x < count)
        dst[id.x] = src[id.x + value];
}
