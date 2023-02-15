package cranelift

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"unsafe"

	"github.com/tetratelabs/wazerolift/internal/wazero/engineext"

	"github.com/tetratelabs/wabin/wasm"
	"github.com/tetratelabs/wazero/api"
)

type (
	compilationContextFunctionIndexKey         struct{}
	compilationContextImportedFunctionCountKey struct{}
	compilationContextModuleKey                struct{}
	compilationContextVmContextOffsetsKey      struct{}
)

func newCompilationContext(m engineext.Module, funcIndex engineext.Index, offsets *opaqueVmContextOffsets) context.Context {
	cmpCtx := context.WithValue(context.Background(), compilationContextModuleKey{}, m)
	cmpCtx = context.WithValue(cmpCtx, compilationContextImportedFunctionCountKey{}, m.ImportFuncCount())
	cmpCtx = context.WithValue(cmpCtx, compilationContextFunctionIndexKey{}, funcIndex)
	cmpCtx = context.WithValue(cmpCtx, compilationContextVmContextOffsetsKey{}, offsets)
	return cmpCtx
}

func (e *engine) addWazeroModule(ctx context.Context) {
	const wazeroModuleName = "wazero"
	_, err := e.craneliftRuntime.NewHostModuleBuilder(wazeroModuleName).
		NewFunctionBuilder().
		WithFunc(e.exportCompileDone).
		Export("compile_done").
		NewFunctionBuilder().
		WithFunc(e.exportFuncIndex).
		Export("func_index").
		NewFunctionBuilder().
		WithFunc(e.exportCurrentFuncTypeIndex).
		Export("current_func_type_index").
		NewFunctionBuilder().
		WithFunc(e.exportFuncTypeIndex).
		Export("func_type_index").
		NewFunctionBuilder().
		WithFunc(e.exportTypeCounts).
		Export("type_counts").
		NewFunctionBuilder().
		WithFunc(e.exportTypeLens).
		Export("type_lens").
		NewFunctionBuilder().
		WithFunc(e.exportTypeParamAt).
		Export("type_param_at").
		NewFunctionBuilder().
		WithFunc(e.exportTypeResultAt).
		Export("type_result_at").
		NewFunctionBuilder().
		WithFunc(e.exportIsLocallyDefinedFunction).
		Export("is_locally_defined_function").
		NewFunctionBuilder().
		WithFunc(e.exportMemoryMinMax).
		Export("memory_min_max").
		NewFunctionBuilder().
		WithFunc(e.exportIsMemoryImported).
		Export("is_memory_imported").
		NewFunctionBuilder().
		WithFunc(e.exportMemoryInstanceBaseOffset).
		Export("memory_instance_base_offset").
		NewFunctionBuilder().
		WithFunc(e.exportVmContextLocalMemoryOffset).
		Export("vm_context_local_memory_offset").
		NewFunctionBuilder().
		WithFunc(e.exportVmContextImportedMemoryOffset).
		Export("vm_context_imported_memory_offset").
		NewFunctionBuilder().
		WithFunc(e.exportVmContextImportedFunctionOffset).
		Export("vm_context_imported_function_offset").
		Instantiate(ctx)
	if err != nil {
		panic(err)
	}
}

func (e *engine) exportCompileDone(ctx context.Context, mod api.Module, codePtr, codeSize, relocsPtr, relocCounts uint32) {
	m := mustModulePtrFromContext(ctx)

	compiled, ok := mod.Memory().Read(codePtr, codeSize)
	if !ok {
		panic(fmt.Sprintf("invalid memory position for compiled body: (ptr=%#x,size=%#x)", codePtr, codeSize))
	}

	// We have to copy the result into Go-allocated slice.
	body := make([]byte, codeSize)
	copy(body, compiled)

	var relocs []functionRelocationEntry
	if relocCounts > 0 {
		relocsEnd := relocCounts * uint32(unsafe.Sizeof(functionRelocationEntry{})) // TODO: make this as const and add assertion test.
		relocsBytes, ok := mod.Memory().Read(relocsPtr, relocsEnd)
		if !ok {
			panic(fmt.Sprintf("invalid memory position for relocs: (ptr=%#x,size=%#x)", relocsPtr, relocsEnd))
		}

		relocInfos := make([]byte, relocsEnd)
		copy(relocInfos, relocsBytes)

		relocs = *(*[]functionRelocationEntry)(unsafe.Pointer(&reflect.SliceHeader{ //nolint
			Data: uintptr(unsafe.Pointer(&relocInfos[0])),
			Len:  int(relocCounts),
			Cap:  int(relocCounts),
		}))
		runtime.KeepAlive(relocInfos)
	}

	// TODO: take mutex lock.
	id := m.ModuleID()
	e.pendingCompiledFunctions[id] = append(e.pendingCompiledFunctions[id], pendingCompiledBody{
		machineCode: body,
		relocs:      relocs,
	})
}

func (e *engine) exportFuncIndex(ctx context.Context, _ api.Module) uint32 {
	return mustFuncIndexFromContext(ctx)
}

func (e *engine) exportCurrentFuncTypeIndex(ctx context.Context, _ api.Module) uint32 {
	m := mustModulePtrFromContext(ctx)
	fidx := mustFuncIndexFromContext(ctx)
	return m.FuncTypeIndex(fidx)
}

func (e *engine) exportFuncTypeIndex(ctx context.Context, _ api.Module, fidx uint32) uint32 {
	m := mustModulePtrFromContext(ctx)
	return m.FuncTypeIndex(fidx)
}

func (e *engine) exportTypeCounts(ctx context.Context, _ api.Module) uint32 {
	m := mustModulePtrFromContext(ctx)
	return m.TypeCounts()
}

func (e *engine) exportTypeLens(ctx context.Context, craneliftMod api.Module, typeIndex, paramLenPtr, resultLenPtr uint32) {
	m := mustModulePtrFromContext(ctx)
	params, results := m.Type(typeIndex)
	mem := craneliftMod.Memory()
	mem.WriteUint32Le(paramLenPtr, uint32(len(params)))
	mem.WriteUint32Le(resultLenPtr, uint32(len(results)))
}

func (e *engine) exportTypeParamAt(ctx context.Context, _ api.Module, typeIndex, at uint32) uint32 {
	m := mustModulePtrFromContext(ctx)
	params, _ := m.Type(typeIndex)
	return valueTypeToCraneliftEnum(params[at])
}

func (e *engine) exportTypeResultAt(ctx context.Context, _ api.Module, typeIndex, at uint32) uint32 {
	m := mustModulePtrFromContext(ctx)
	_, results := m.Type(typeIndex)
	return valueTypeToCraneliftEnum(results[at])
}

func (e *engine) exportIsLocallyDefinedFunction(ctx context.Context, _ api.Module, funcIndex uint32) uint32 {
	m := mustModulePtrFromContext(ctx)
	cnt := m.ImportFuncCount()
	if cnt <= funcIndex {
		return 1
	} else {
		return 0
	}
}

func (e *engine) exportMemoryMinMax(ctx context.Context, craneliftMod api.Module, minPtr, maxPtr uint32) uint32 {
	m := mustModulePtrFromContext(ctx)
	min, max, ok := m.MemoryMinMax()
	if !ok {
		return 0
	}

	mem := craneliftMod.Memory()
	mem.WriteUint32Le(minPtr, min)
	mem.WriteUint32Le(maxPtr, max)
	return 1
}

func (e *engine) exportIsMemoryImported(ctx context.Context, _ api.Module) uint32 {
	m := mustModulePtrFromContext(ctx)
	if m.ImportedMemoriesCount() > 0 {
		return 1
	} else {
		return 0
	}
}

func (e *engine) exportVmContextLocalMemoryOffset(ctx context.Context, _ api.Module) uint32 {
	offsets := mustVmContextOffsetsFromContext(ctx)
	return uint32(offsets.localMemoryBegin)
}

func (e *engine) exportMemoryInstanceBaseOffset() uint32 {
	return engineext.MemoryInstanceBufferOffset
}

func (e *engine) exportVmContextImportedMemoryOffset(ctx context.Context, _ api.Module) uint32 {
	offsets := mustVmContextOffsetsFromContext(ctx)
	return uint32(offsets.importedMemoryBegin)
}

func (e *engine) exportVmContextImportedFunctionOffset(ctx context.Context, _ api.Module, index uint32) uint32 {
	offsets := mustVmContextOffsetsFromContext(ctx)
	return uint32(offsets.importedFunctionsBegin + int(index)*16)
}

func valueTypeToCraneliftEnum(v wasm.ValueType) uint32 {
	switch v {
	case wasm.ValueTypeI32:
		return 0
	case wasm.ValueTypeI64:
		return 1
	case wasm.ValueTypeF32:
		return 2
	case wasm.ValueTypeF64:
		return 3
	case wasm.ValueTypeV128:
		return 4
	case wasm.ValueTypeFuncref:
		return 5
	case wasm.ValueTypeExternref:
		return 6
	default:
		panic("BUG")
	}
}

func mustModulePtrFromContext(ctx context.Context) engineext.Module {
	m, ok := ctx.Value(compilationContextModuleKey{}).(engineext.Module)
	if !ok {
		panic("BUG: invalid compilation context without *wasm.Module")
	}
	return m
}

func mustFuncIndexFromContext(ctx context.Context) uint32 {
	ret, ok := ctx.Value(compilationContextFunctionIndexKey{}).(wasm.Index)
	if !ok {
		panic("BUG: invalid compilation context without func id")
	}
	return ret
}

func mustVmContextOffsetsFromContext(ctx context.Context) *opaqueVmContextOffsets {
	offsets, ok := ctx.Value(compilationContextVmContextOffsetsKey{}).(*opaqueVmContextOffsets)
	if !ok {
		panic("BUG: invalid compilation context without *opaqueVmContextOffsets")
	}
	return offsets
}

func mustImportedFunctionCountFromContext(ctx context.Context) uint32 {
	counts, ok := ctx.Value(compilationContextImportedFunctionCountKey{}).(uint32)
	if !ok {
		panic("BUG: invalid compilation context without imported function counts")
	}
	return counts
}
