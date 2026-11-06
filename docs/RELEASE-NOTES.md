# Shomer Live — offline TCL Flip 2 workbench

Download all ISO `.part-*` files, `download-manifest.json` and `Join-ISO.ps1` into
one folder. Run the PowerShell script to reassemble and SHA-256 verify the ISO.
No compilation or Python is needed on Windows. Write the resulting ISO with Rufus
in DD mode; the selected USB stick is erased. See `DOWNLOAD-INSTRUCTIONS.txt`.

Includes Debian 13 + a minimal Xfce desktop, the current Shomer ROM, stock KEKA and
KEE7 reference firmware, Neutronscott images/tools, MTKClient CLI/GUI with complete
offline dependencies/loaders/payloads, Android platform-tools and image utilities.
The separate Windows flasher executable uses the same engine and GUI presentation.

The guided workflow prefers read-only MTKClient BROM backup when available;
recovery backup is a separate fallback. Mode transitions and recovery size are
verified before writes. Backups/logs use explicitly selected writable storage.
No production private signing keys or customer backups are included.

Publication requires the release workflow's Linux, Windows and offline BIOS/UEFI
VM gates. Read `test-report.json` and `VALIDATION.md` for scope and evidence.
Physical Rufus USB boot, Secure Boot and real-phone flashing/radio tests are NOT
claimed. BROM access depends on the particular phone/firmware. This prerelease
is for controlled testing; it is not a promise that every TCL/4058 device works.
