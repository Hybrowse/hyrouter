package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/hybrowse/hyrouter/internal/config"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type wasmPlugin struct {
	name   string
	rt     wazero.Runtime
	mod    api.Module
	alloc  api.Function
	onConn api.Function
	logger *slog.Logger
}

func newWASMPlugin(ctx context.Context, cfg config.PluginConfig, logger *slog.Logger) (Plugin, error) {
	if cfg.WASM == nil || cfg.WASM.Path == "" {
		return nil, fmt.Errorf("wasm.path must not be empty")
	}
	b, err := os.ReadFile(cfg.WASM.Path)
	if err != nil {
		return nil, err
	}

	rt := wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	compiled, err := rt.CompileModule(ctx, b)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}
	modCfg := wazero.NewModuleConfig().WithStartFunctions("_initialize")
	mod, err := rt.InstantiateModule(ctx, compiled, modCfg)
	if err != nil {
		mod, err = rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	}
	if err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}

	alloc := mod.ExportedFunction("alloc")
	if alloc == nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("missing export: alloc")
	}
	onConn := mod.ExportedFunction("on_connect")
	if onConn == nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("missing export: on_connect")
	}

	return &wasmPlugin{name: cfg.Name, rt: rt, mod: mod, alloc: alloc, onConn: onConn, logger: logger}, nil
}

func (p *wasmPlugin) Name() string { return p.name }

func (p *wasmPlugin) OnConnect(ctx context.Context, req ConnectRequest) (ConnectResponse, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return ConnectResponse{}, err
	}

	res, err := p.alloc.Call(ctx, uint64(len(b)))
	if err != nil {
		return ConnectResponse{}, err
	}
	ptr := uint32(res[0])
	if !p.mod.Memory().Write(ptr, b) {
		return ConnectResponse{}, fmt.Errorf("memory write failed")
	}

	out, err := p.onConn.Call(ctx, uint64(ptr), uint64(len(b)))
	if err != nil {
		return ConnectResponse{}, err
	}
	packed := out[0]
	respPtr := uint32(packed >> 32)
	respLen := uint32(packed & 0xffffffff)
	respBytes, ok := p.mod.Memory().Read(respPtr, respLen)
	if !ok {
		return ConnectResponse{}, fmt.Errorf("memory read failed")
	}

	var resp ConnectResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return ConnectResponse{}, err
	}
	return resp, nil
}

func (p *wasmPlugin) Close(ctx context.Context) error {
	if p.rt == nil {
		return nil
	}
	return p.rt.Close(ctx)
}
