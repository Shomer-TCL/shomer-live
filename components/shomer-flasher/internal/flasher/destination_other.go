//go:build !linux

package flasher

func checkBackupDestination(dir string) error { return nil }
