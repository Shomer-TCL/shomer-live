# Shomer Live

An offline Debian 13 amd64 live USB workbench for TCL Flip 2 development,
read-only backup and installation of the pinned Shomer image.

No installer runs automatically. Internal disks are not mounted by this toolkit.
Phone writes require the flasher's review and explicit confirmation. Prefer the
read-only MTKClient BROM backup route when the particular phone exposes BROM;
recovery backup is a separate fallback and overwrites recovery before reading.

## Build

The GitHub Actions release workflow assembles all dependencies on an online
runner, builds the BIOS/UEFI hybrid ISO, boots it without a network card, runs the
offline checks and GUI tests, and publishes a prerelease only if those gates pass.
See [BUILDING.md](docs/BUILDING.md), [TESTING.md](docs/TESTING.md), and the release
asset inventory. Debian package versions are recorded for each build; identical
ISO bytes across different Debian mirror updates are not promised.

The `assets` release contains the private stock/current ROM source archives.
Their checksums and provenance are pinned in `config/assets.lock.json`.
Production private signing keys, customer data and per-device backups are never
build inputs. Only the current Shomer release and KEKA/KEE7 stock images are bundled.

## Use

Download the release files, verify/reassemble with the supplied Windows PowerShell
or Linux script, then write the resulting ISO with Rufus in DD mode or another
raw-image writer. Boot the USB and open **Shomer Workbench**. Choose a writable
backup drive explicitly. RAM-backed live storage is rejected for backups.

Physical Rufus/PC/Secure Boot and phone flashing tests require hardware and are
reported separately from automated VM tests. This project does not claim support
for every ROM, bootloader, BROM state or 4058-family model.
