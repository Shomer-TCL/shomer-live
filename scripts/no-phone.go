// No-phone tool for GUI tests: accepts read-only probes and refuses every other command.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(1)
	}
	switch os.Args[1] {
	case "--version":
		fmt.Println("fastboot version 36.0.0")
	case "version":
		fmt.Println("Android Debug Bridge version 1.0.41")
	case "devices", "kill-server":
		// Intentionally no devices. No USB library or process execution is linked.
	default:
		os.Exit(1)
	}
}
