// Copyright (c) the go-mswin/d3d11 authors.
// SPDX-License-Identifier: BSD-3-Clause

package d3d11

// Vtable slots, in declaration order of d3d11.h, base interfaces first. A wrong
// slot does not crash: it calls a different method. Each list is written out
// in full so that the number of a method is its position in the list, and
// slots_test.go pins anchors (the methods go-mswin/screencapture already uses,
// and the first and last of each list). The dispatch test on WARP is the
// end-to-end proof.

// ID3D11Device: IUnknown QueryInterface(0) AddRef(1) Release(2), then:
const (
	devCreateBuffer = iota + 3
	devCreateTexture1D
	devCreateTexture2D
	devCreateTexture3D
	devCreateShaderResourceView
	devCreateUnorderedAccessView
	devCreateRenderTargetView
	devCreateDepthStencilView
	devCreateInputLayout
	devCreateVertexShader
	devCreateGeometryShader
	devCreateGeometryShaderWithStreamOutput
	devCreatePixelShader
	devCreateHullShader
	devCreateDomainShader
	devCreateComputeShader
	devCreateClassLinkage
	devCreateBlendState
	devCreateDepthStencilState
	devCreateRasterizerState
	devCreateSamplerState
	devCreateQuery
	devCreatePredicate
	devCreateCounter
	devCreateDeferredContext
	devOpenSharedResource
	devCheckFormatSupport
	devCheckMultisampleQualityLevels
	devCheckCounterInfo
	devCheckCounter
	devCheckFeatureSupport
	devGetPrivateData
	devSetPrivateData
	devSetPrivateDataInterface
	devGetFeatureLevel
	devGetCreationFlags
	devGetDeviceRemovedReason
	devGetImmediateContext
	devSetExceptionMode
	devGetExceptionMode
)

// ID3D11DeviceContext: IUnknown(0..2), ID3D11DeviceChild: GetDevice(3)
// GetPrivateData(4) SetPrivateData(5) SetPrivateDataInterface(6), then:
const (
	ctxVSSetConstantBuffers = iota + 7
	ctxPSSetShaderResources
	ctxPSSetShader
	ctxPSSetSamplers
	ctxVSSetShader
	ctxDrawIndexed
	ctxDraw
	ctxMap
	ctxUnmap
	ctxPSSetConstantBuffers
	ctxIASetInputLayout
	ctxIASetVertexBuffers
	ctxIASetIndexBuffer
	ctxDrawIndexedInstanced
	ctxDrawInstanced
	ctxGSSetConstantBuffers
	ctxGSSetShader
	ctxIASetPrimitiveTopology
	ctxVSSetShaderResources
	ctxVSSetSamplers
	ctxBegin
	ctxEnd
	ctxGetData
	ctxSetPredication
	ctxGSSetShaderResources
	ctxGSSetSamplers
	ctxOMSetRenderTargets
	ctxOMSetRenderTargetsAndUnorderedAccessViews
	ctxOMSetBlendState
	ctxOMSetDepthStencilState
	ctxSOSetTargets
	ctxDrawAuto
	ctxDrawIndexedInstancedIndirect
	ctxDrawInstancedIndirect
	ctxDispatch
	ctxDispatchIndirect
	ctxRSSetState
	ctxRSSetViewports
	ctxRSSetScissorRects
	ctxCopySubresourceRegion
	ctxCopyResource
	ctxUpdateSubresource
	ctxCopyStructureCount
	ctxClearRenderTargetView
	ctxClearUnorderedAccessViewUint
	ctxClearUnorderedAccessViewFloat
	ctxClearDepthStencilView
	ctxGenerateMips
	ctxSetResourceMinLOD
	ctxGetResourceMinLOD
	ctxResolveSubresource
	ctxExecuteCommandList
	ctxHSSetShaderResources
	ctxHSSetShader
	ctxHSSetSamplers
	ctxHSSetConstantBuffers
	ctxDSSetShaderResources
	ctxDSSetShader
	ctxDSSetSamplers
	ctxDSSetConstantBuffers
	ctxCSSetShaderResources
	ctxCSSetUnorderedAccessViews
	ctxCSSetShader
	ctxCSSetSamplers
	ctxCSSetConstantBuffers
	ctxVSGetConstantBuffers
	ctxPSGetShaderResources
	ctxPSGetShader
	ctxPSGetSamplers
	ctxVSGetShader
	ctxPSGetConstantBuffers
	ctxIAGetInputLayout
	ctxIAGetVertexBuffers
	ctxIAGetIndexBuffer
	ctxGSGetConstantBuffers
	ctxGSGetShader
	ctxIAGetPrimitiveTopology
	ctxVSGetShaderResources
	ctxVSGetSamplers
	ctxGetPredication
	ctxGSGetShaderResources
	ctxGSGetSamplers
	ctxOMGetRenderTargets
	ctxOMGetRenderTargetsAndUnorderedAccessViews
	ctxOMGetBlendState
	ctxOMGetDepthStencilState
	ctxSOGetTargets
	ctxRSGetState
	ctxRSGetViewports
	ctxRSGetScissorRects
	ctxHSGetShaderResources
	ctxHSGetShader
	ctxHSGetSamplers
	ctxHSGetConstantBuffers
	ctxDSGetShaderResources
	ctxDSGetShader
	ctxDSGetSamplers
	ctxDSGetConstantBuffers
	ctxCSGetShaderResources
	ctxCSGetUnorderedAccessViews
	ctxCSGetShader
	ctxCSGetSamplers
	ctxCSGetConstantBuffers
	ctxClearState
	ctxFlush
	ctxGetType
	ctxGetContextFlags
	ctxFinishCommandList
)
