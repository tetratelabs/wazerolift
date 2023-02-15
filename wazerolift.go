package wazerolift

import (
	"context"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	cranelift "github.com/tetratelabs/wazerolift/internal"
	"github.com/tetratelabs/wazerolift/internal/wazero/engineext"
)

func ConfigureCranelift(config wazero.RuntimeConfig) {
	// This is the internal representation of interface in Go.
	// https://research.swtch.com/interfaces
	type iface struct {
		tp   *byte
		data unsafe.Pointer
	}

	configInterface := (*iface)(unsafe.Pointer(&config))
	if configInterface == nil {
		panic("BUG: invalid configuration was given")
	}

	// This corresponds to the unexported wazero.runtimeConfig, and set the target field newEngineExt exists
	// at the beginning of the implementation. That assumption is ensured by wazero's unit test.
	type newEngineExt func(context.Context, api.CoreFeatures, any) engineext.EngineExt
	type runtimeConfig struct {
		newEngineExt
	}
	cm := (*runtimeConfig)(configInterface.data)
	// Insert the cranelift implementation.
	cm.newEngineExt = cranelift.NewEngine
}
