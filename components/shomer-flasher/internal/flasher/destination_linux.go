//go:build linux

package flasher

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// The live ISO must retain backups after shutdown, including destinations edited
// inside either UI. Ordinary installed Linux systems keep their usual behavior.
func checkBackupDestination(dir string) error {
	if _, err := os.Stat("/etc/shomer-live"); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	out, err := exec.Command("/usr/local/bin/shomer-check-destination", dir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("backup destination is not persistent storage: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}
