//go:build windows

package main

import "unsafe"

func uintptrOf(p *uint32) uintptr { return uintptr(unsafe.Pointer(p)) }
