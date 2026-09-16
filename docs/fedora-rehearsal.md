# Fedora Rehearsal

Fedora support is verified along two separable axes, and only one of them is worth automating. Per **ADR-15**,
distro variance is cheap to automate — so it runs on every pull request as the `fedora` job in both Go workflows —
while architecture variance and anything that needs PID 1 is rehearsed locally, by hand, in the two containers
described below. No arm64 runner is bought: the `dotf` binary is a CGO-free cross-compile with no arch-specific
codegen, and arm64 **plus** systemd together is unreachable under emulation anyway, so a hosted arm64 runner would
buy far less than its price. Everything neither CI nor a container can settle is deferred to the physical machine
via [`docs/fedora-first-run.md`](fedora-first-run.md).

Run the containers before opening a Fedora-touching pull request, not instead of the checklist.

## What CI already covers

The `fedora` job in `.github/workflows/go.yml` and `.forgejo/workflows/go.yml` runs inside
`container: fedora:latest`, against a real `/etc/os-release`, a real dnf5 and a real rpm database. It runs:

- `go vet ./...`
- `go test -race ./...`
- a smoke step asserting that `dotf platform` output contains `os=redhat` and `pkg=dnf`, and that
  `dotf apply --dry-run --headless` and `dotf link --dry-run` both exit 0.

Alongside it, `shell.yml` on both forges runs shellcheck over every tracked `*.sh` through the repo's single style
dispatcher (`bin/lint.sh --shell`).

Nothing else in the repository executes against a real Fedora userspace. Every other Fedora assertion in the tree —
detection, manifest applicability, backend argv, sysconfig planning — runs against `exec.Fake`, which is exactly why
the two containers below exist.

## Container A — native amd64, privileged, systemd

This is the container for the `dotf sysconfig` domains that need a running init: firewalld, sshd and localectl. It
is native amd64, so it is fast; it is privileged with the host cgroup namespace, so `systemd` can actually be PID 1.

```bash
docker run -d --name dotf-fedora-systemd \
  --privileged --cgroupns=host \
  -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
  fedora:latest \
  bash -c 'dnf -y install --setopt=install_weak_deps=False systemd dbus-broker firewalld openssh-server && exec /usr/lib/systemd/systemd'

# give PID 1 ~45s, then:
docker exec dotf-fedora-systemd systemctl enable --now dbus-broker
docker exec dotf-fedora-systemd systemctl restart firewalld
docker exec dotf-fedora-systemd firewall-cmd --state
```

Measured behaviour, recorded so nobody re-derives it:

- `systemctl is-system-running` reports `degraded`. That is expected inside a container and it is **not** a
  problem to chase — units that need real hardware or a real network stack will not start, and the domains this
  container exists to exercise come up regardless.
- **`dbus-broker` is the trap.** `fedora:latest` does not pull it in transitively, and without it installed _and_
  started, `firewall-cmd --state` fails with
  `DBUS_ERROR: Failed to connect to socket /run/dbus/system_bus_socket`. This is the single most likely thing to
  waste an hour here. It is in the `dnf` list above on purpose.
- Once `dbus-broker` is up: `firewall-cmd --state` prints `running`, `firewall-cmd --get-default-zone` prints
  `public`, `systemctl enable --now sshd` leaves the unit `active`, and
  `firewall-cmd --permanent --query-service=ssh` prints `yes`. Those are the same reads
  `internal/sysconfig/linux.go` uses to decide whether the firewall and ssh-server domains need any Steps at all,
  so a real `dotf sysconfig --timezone … --firewall firewalld --ssh-server on --yes` can be driven to `failed=0`
  in here.
- `hostnamectl set-hostname rehearsal` **fails**, with `Could not set static hostname: … Device or resource busy`.
  `/etc/hostname` is owned by the container runtime, so the hostname domain is a physical-device-only claim no
  matter how the container is configured. Do not try to work around it; move it to the device checklist.
- A container's firewalld zone is not a roaming laptop's zone. A green run here says the argv and the plan are
  right; it says nothing about what zone NetworkManager will pick on real Wi-Fi.

Cleanup:

```bash
docker rm -f dotf-fedora-systemd
```

## Container B — emulated aarch64

This is the container for the architecture axis: package availability and release-asset matching on aarch64. It
runs under `qemu-user` via binfmt, so it has no usable init — do not try to combine it with Container A.

```bash
# one-time, per boot
docker run --privileged --rm tonistiigi/binfmt --install arm64

docker run --rm --platform linux/arm64 fedora:latest \
  sh -c 'uname -m; cat /etc/os-release | head -3'
# -> aarch64 / NAME="Fedora Linux" / VERSION="44 (Container Image)"
```

Expect roughly an 8–9× wall-clock penalty on anything in the RPM layer (`dnf` transactions, `rpm -q`) and
effectively none on the Go binary itself — it is a CGO-free cross-compile with no arch-specific codegen, so the
emulator has nothing interesting to slow down. Use this container for a real multi-package `dnf` transaction and
for confirming that `rpm -q` agrees with what the installed-check expects on aarch64.

## What no container can prove

These are the accepted residual risks. Each has a numbered step in
[`docs/fedora-first-run.md`](fedora-first-run.md), which is the device checklist and the acceptance gate — walk it
end to end on the actual M1 before cutting the release tag.

- **arm64 plus systemd together.** Measured: `dbus-broker` dies `226/NAMESPACE` under `qemu-user`. The only path
  that would work is a full `qemu-system-aarch64` TCG VM, which ADR-15 puts explicitly out of scope.
- **SELinux.** No container on an x86 host exposes `/sys/fs/selinux`; the checklist records `getenforce` before and
  after.
- **`hostnamectl set-hostname`.** Blocked in Container A as described above.
- **btrfs and snapper.** Containers are overlayfs; the snapshot path is exercised only through the fake.
- **The 16 KiB page size** and its interaction with any C-linked third-party binary. Not modelled by any x86
  container.
- **A live KDE Plasma 6 session** — how the merge actually looks on a Retina display, and what `--reload`'s
  component restarts do to a running desktop. `dotf plasma diff` runs headless, but the visible result cannot be
  judged headlessly.
- **`bootstrap.sh` against a private prerelease origin.** No CI job exercises the shim against one.

## Reproducing the CI Fedora job locally

This is the same recipe the `fedora` CI job runs, condensed to one command. It exits 0 and prints `FEDORA_JOB_OK`
on success.

```bash
docker run --rm -v "$PWD":/w:ro -v "$(go env GOROOT)":/usr/local/go:ro \
  -e PATH=/usr/local/go/bin:/usr/local/sbin:/usr/sbin:/sbin:/usr/local/bin:/usr/bin:/bin \
  fedora:latest bash -c 'set -euo pipefail; dnf -y install --setopt=install_weak_deps=False git gcc tar gzip findutils diffutils >/dev/null 2>&1; cp -a /w /work; cd /work; go vet ./...; go test -race ./...; go build -o /tmp/dotf ./cmd/dotf; out="$(/tmp/dotf platform)"; echo "$out"; case "$out" in *os=redhat*) ;; *) exit 1 ;; esac; case "$out" in *pkg=dnf*) ;; *) exit 1 ;; esac; /tmp/dotf apply --dry-run --headless >/dev/null; /tmp/dotf link --dry-run >/dev/null; echo FEDORA_JOB_OK'
```

The observed `dotf platform` line is:

```text
os=redhat arch=x86_64 pkg=dnf container=docker distro=fedora
```

Record it, and compare against it — the whole point of printing it is that drift is visible.

Two details are load-bearing rather than incidental:

- The worktree is mounted **read-only** at `/w` and then copied with `cp -a` to `/work` before anything runs. The
  build writes object files and the test suite writes temporary trees, so the container needs a writable copy —
  and mounting the checkout read-write would let a rehearsal mutate the developer's working tree. It cannot.
- `git`, `gcc`, `tar`, `gzip`, `findutils` and `diffutils` are installed explicitly, matching the CI job's
  prerequisite step. `fedora:latest` ships dnf5, rpm, bash, curl, tar and sudo but **not** git or gcc; the archive
  and file utilities ride along so the list is identical to CI's. Without git, `actions/checkout` silently degrades
  to a REST tarball; without gcc, `go test -race` fails with `-race requires cgo`.
