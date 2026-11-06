# Maintainer build instructions

End users download the built ISO from Releases. These instructions are for maintainers.

Debian 13 live-build runs on a Linux amd64 GitHub runner with root privileges.
The online build stage verifies locked firmware chunks, upstream downloads and
hash-locked Python wheels before creating the offline filesystem. The final
chroot installs MTKClient dependencies exclusively from the wheelhouse.

1. Install the build dependencies listed in `.github/workflows/release.yml`.
2. Check out `config/assets.lock.json:git_ref` from this same private repository
   into `build/input-chunks`. This branch contains large private firmware inputs.
3. Run `python3 scripts/stage.py --inputs build/input-chunks`.
4. Run `bash scripts/build-iso.sh`; it configures and builds inside the pinned privileged Debian build container.
5. Run `python3 scripts/test-iso.py build/live/live-image-amd64.hybrid.iso`.
6. Package with `python3 scripts/package-release.py <ISO> --version <VERSION>`.

The release workflow performs these steps and publishes only after Linux and
Windows tests pass. It never copies this task's `.setup` directory, credentials,
signing keys or personal partition backups into the ISO.

Update pins deliberately. Do not edit expected hashes to accept an unexplained
download mismatch. The Debian base is pinned to the stable release; package
versions are recorded, and mirror updates can change subsequent build bytes.
For byte-identical rebuilds, retain the ISO, manifests and exact package artifacts.

The vendored `components/shomer-flasher` is exported from the feature commit named
in `config/sources.lock.json`; the same engine produces Linux GTK, Linux terminal
and Windows GUI interfaces. Source changes must be synchronized with the original
shomer-flasher repository, then repinned. MTKClient source includes loaders and
payloads; its locked wheels are installed for Debian's Python 3.13.
