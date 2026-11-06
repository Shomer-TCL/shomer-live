# Validation and honest limits

Required automated gates:

- Go vet, unit/regression tests and Linux race tests for the shared flashing engine.
- Unknown/unsupported modes, failed transitions, recovery-size mismatch, failed
  erase/reboot, and separation of entered vs detected identity.
- CLI and terminal mock-phone end-to-end suites, with actual payload checksums,
  backup ZIP verification and recovery-restore ordering.
- GTK rendering of all shared states and real X11 input: default-No confirmation
  performs no writes; explicit Yes completes a simulated backup and installation.
- Windows native GUI rendering, dialog self-tests and real input end-to-end tests.
- Tool/dependency imports, complete loader/payload inventory and forbidden-secret scan.
- Actual release ISO booted through BIOS and UEFI with `-nic none`.
- Actual image inspection, full bundled payload verification/unpacking, GTK and
  MTKClient startup, and mock-phone suites inside the offline live system.
- Shut down and boot again with the same labeled test disk; verify retained data.

QEMU `fw_cfg` enables the self-test service, which requires the explicitly labeled
`SHOMER_TEST_BACKUPS` test volume. No altered test ISO or bypassed bootloader is
used. The regular physical live session never runs that service.

Automated evidence does **not** establish real BROM support, phone boot/radio
behavior, successful real-phone flashing, physical Rufus USB boot, or Secure Boot
compatibility. Release notes and `test-report.json` keep these unrun checks separate.
Do not claim the TAG ROM caused a particular failure without device evidence.

The BROM helper only runs MTKClient storage reads. It requires BROM enumeration,
positive eMMC geometry and two equal full reads of user/boot1/boot2. RPMB/efuses are
excluded. A rooted recovery backup is explicitly different: it needs unlock and
overwrites recovery first; encrypted userdata may not be recoverable as user files.
