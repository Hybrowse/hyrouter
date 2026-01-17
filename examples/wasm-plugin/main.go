//go:build examples

package main

import (
	"sync"
	"unsafe"
)

var (
	allocMu sync.Mutex
	allocs  = map[uint32][]byte{}
)

//go:wasmexport alloc
func Alloc(size uint32) uint32 {
	if size == 0 {
		return 0
	}
	b := make([]byte, size)
	ptr := uint32(uintptr(unsafe.Pointer(&b[0])))
	allocMu.Lock()
	allocs[ptr] = b
	allocMu.Unlock()
	return ptr
}

//go:wasmexport on_connect
func OnConnect(ptr uint32, length uint32) uint64 {
	in := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
	deny := contains(in, []byte("\"username\":\"deny\""))
	if deny {
		out := []byte(`{"deny":true,"deny_reason":"denied by wasm plugin"}`)
		p := Alloc(uint32(len(out)))
		copy(unsafe.Slice((*byte)(unsafe.Pointer(uintptr(p))), uint32(len(out))), out)
		return (uint64(p) << 32) | uint64(len(out))
	}

	out := []byte(`{"referral_data":"d2FzbS1wbHVnaW4=","target":{"host":"play.hyvane.com","port":5520}}`)
	p := Alloc(uint32(len(out)))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(uintptr(p))), uint32(len(out))), out)
	return (uint64(p) << 32) | uint64(len(out))
}

func contains(hay []byte, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		ok := true
		for j := 0; j < len(needle); j++ {
			if hay[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func main() {}
