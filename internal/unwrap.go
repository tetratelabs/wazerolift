package cranelift

import "github.com/tetratelabs/wazerolift/internal/wazero/engineext"

func MustUnwrapModule(raw any) engineext.Module {
	return raw.(engineext.Module)
}

func MustUnwrapModuleInstance(raw any) engineext.ModuleInstance {
	return raw.(engineext.ModuleInstance)
}

func MustUnwrapFunctionInstance(raw any) engineext.FunctionInstance {
	return raw.(engineext.FunctionInstance)
}
