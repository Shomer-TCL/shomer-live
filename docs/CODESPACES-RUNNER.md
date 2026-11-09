# Codespaces self-hosted runner

The Linux `test`, ISO `build`, and `release` jobs target a runner with the label
`shomer-live-codespace`. Windows-native source and release checks remain on
`windows-latest` and need a Windows runner.

Use a Codespace with at least 4 cores, 16 GB RAM, and 32 GB storage for the
full image build. A smaller Codespace may run out of memory or disk while
building the ISO. Codespaces without `/dev/kvm` use QEMU TCG automatically;
the current VM test has a 25-minute limit, so software emulation may exceed the
limit. A passing Codespaces run has not yet been established.

In the repository's **Settings → Actions → Runners**, add a new repository
runner and copy its one-hour registration token directly into the Codespace
terminal when prompted. Do not put the token in a commit, Codespaces secret,
or chat message. The runner uses the default `self-hosted`, `linux`, and `x64`
labels plus `shomer-live-codespace`.

Run these commands in the Codespace terminal:

```bash
mkdir -p "$HOME/actions-runner"
cd "$HOME/actions-runner"
RUNNER_VERSION="$(curl -fsSL https://api.github.com/repos/actions/runner/releases/latest | python3 -c 'import json,sys; print(json.load(sys.stdin)["tag_name"].lstrip("v"))')"
curl -fsSL "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" -o actions-runner.tar.gz
tar -xzf actions-runner.tar.gz
sudo ./bin/installdependencies.sh
read -rsp 'Repository runner registration token: ' RUNNER_REGISTRATION_TOKEN
printf '\n'
./config.sh --url https://github.com/Shomer-TCL/shomer-live --token "$RUNNER_REGISTRATION_TOKEN" --name shomer-live-codespace --labels shomer-live-codespace --unattended
unset RUNNER_REGISTRATION_TOKEN
./run.sh
```

Keep the Codespace awake while jobs are queued or running. This foreground
runner can process the Linux jobs sequentially. It does not run the
`windows-latest` jobs; those remain blocked until Windows-hosted Actions billing
is restored or a separate Windows self-hosted runner is provided.
