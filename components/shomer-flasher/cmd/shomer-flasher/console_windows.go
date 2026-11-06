//go:build windows

package main

import "syscall"

var hiddenConsole uintptr

// hideOwnConsole hides the console window when this program owns it alone,
// i.e. it was double-clicked (started from cmd/PowerShell it is shared and stays).
func hideOwnConsole() {
	k := syscall.NewLazyDLL("kernel32.dll")
	u := syscall.NewLazyDLL("user32.dll")
	var pids [4]uint32
	n, _, _ := k.NewProc("GetConsoleProcessList").Call(uintptrOf(&pids[0]), 4)
	if n != 1 {
		return
	}
	hwnd, _, _ := k.NewProc("GetConsoleWindow").Call()
	if hwnd != 0 {
		u.NewProc("ShowWindow").Call(hwnd, 0) // SW_HIDE
		hiddenConsole = hwnd
	}
}

// showOwnConsole brings the console back (the window could not open).
func showOwnConsole() {
	if hiddenConsole != 0 {
		syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow").Call(hiddenConsole, 5) // SW_SHOW
	}
}
