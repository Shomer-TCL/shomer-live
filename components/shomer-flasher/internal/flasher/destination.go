package flasher

import "os"

var backupDestinationCheckFn = checkBackupDestination

func (f *Flow) validateBackupDestination() error {
	if !f.Opts.Backup || f.Opts.RestoreRecovery {
		return nil
	}
	if err := os.MkdirAll(f.Opts.BackupDir, 0o755); err != nil {
		return err
	}
	return backupDestinationCheckFn(f.Opts.BackupDir)
}
