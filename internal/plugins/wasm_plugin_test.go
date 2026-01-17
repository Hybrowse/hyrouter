package plugins

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hybrowse/hyrouter/internal/config"
)

func TestWASMPlugin_OnConnect(t *testing.T) {
	if os.Getenv("GO_WASM_SKIP") != "" {
		t.Skip("GO_WASM_SKIP set")
	}

	tmp := t.TempDir()
	modDir := filepath.Join(tmp, "mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	goMod := []byte("module example.com/plug\n\ngo 1.25.0\n")
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	src := `package main

import (
	"sync"
	"unsafe"
)

var (
	mu sync.Mutex
	allocs = map[uint32][]byte{}
)

//go:wasmexport alloc
func Alloc(size uint32) uint32 {
	if size == 0 {
		return 0
	}
	b := make([]byte, size)
	ptr := uint32(uintptr(unsafe.Pointer(&b[0])))
	mu.Lock()
	allocs[ptr] = b
	mu.Unlock()
	return ptr
}

//go:wasmexport on_connect
func OnConnect(ptr uint32, length uint32) uint64 {
	in := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
	_ = in
	out := []byte(` + "`" + `{"referral_content":"eA=="}` + "`" + `)
	p := Alloc(uint32(len(out)))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(uintptr(p))), uint32(len(out))), out)
	return (uint64(p) << 32) | uint64(len(out))
}

func main() {}
`
	if err := os.WriteFile(filepath.Join(modDir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	wasmPath := filepath.Join(tmp, "plugin.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", wasmPath, ".")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	cmd.Dir = modDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build wasm: %v\n%s", err, string(out))
	}

	p, err := newWASMPlugin(context.Background(), config.PluginConfig{Name: "w", Type: "wasm", WASM: &config.WASMPluginConfig{Path: wasmPath}}, nil)
	if err != nil {
		t.Fatalf("newWASMPlugin: %v", err)
	}
	if p.Name() != "w" {
		t.Fatalf("name=%q", p.Name())
	}
	defer p.Close(context.Background()) // nolint:errcheck

	resp, err := p.OnConnect(context.Background(), ConnectRequest{Event: ConnectEvent{Username: "u"}})
	if err != nil {
		t.Fatalf("OnConnect: %v", err)
	}
	if string(resp.ReferralContent) != "x" {
		t.Fatalf("ref=%q", string(resp.ReferralContent))
	}
}

func TestWASMPlugin_Errors(t *testing.T) {
	if _, err := newWASMPlugin(context.Background(), config.PluginConfig{Name: "w", Type: "wasm"}, nil); err == nil {
		t.Fatalf("expected error")
	}

	bad := filepath.Join(t.TempDir(), "bad.wasm")
	if err := os.WriteFile(bad, []byte("notwasm"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := newWASMPlugin(context.Background(), config.PluginConfig{Name: "w", Type: "wasm", WASM: &config.WASMPluginConfig{Path: bad}}, nil); err == nil {
		t.Fatalf("expected error")
	}

	tmp := t.TempDir()
	modDir := filepath.Join(tmp, "mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	goMod := []byte("module example.com/plug\n\ngo 1.25.0\n")
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	src := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(modDir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	wasmPath := filepath.Join(tmp, "noexports.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", wasmPath, ".")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	cmd.Dir = modDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build wasm: %v\n%s", err, string(out))
	}
	if _, err := newWASMPlugin(context.Background(), config.PluginConfig{Name: "w", Type: "wasm", WASM: &config.WASMPluginConfig{Path: wasmPath}}, nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWASMPlugin_CloseNilRuntime(t *testing.T) {
	p := &wasmPlugin{}
	if err := p.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
