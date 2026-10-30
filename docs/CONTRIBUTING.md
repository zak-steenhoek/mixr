# Contributing to mixr

mixr is a Windows-focused evolution of deej. Keep changes narrow, testable, and
compatible with the validated four-pot and `mixr:v1:` hardware paths. Product
scope is maintained in [REQUIREMENTS.md](REQUIREMENTS.md), architecture choices
in [DECISIONS.md](DECISIONS.md), and repository-specific working rules in the
root [AGENTS.md](../AGENTS.md).

## Development workflow

1. Branch focused implementation separately from experiments.
2. Preserve `config.yaml`, `deej.current`, `deej.unmapped`, the 9600-baud
   pipe-delimited potentiometer stream, and existing `mixr:v1:` messages unless
   an approved migration explicitly changes them.
3. Use `internal/wincom` for audited Core Audio calls; do not restore unsafe
   pinned `go-wca` methods.
4. Run the 386/amd64 commands in `AGENTS.md` from the repository root.
5. Treat physical Windows/Nano validation as distinct from automated tests and
   record the hardware, Windows/build version, procedure, results, and gaps.
6. Keep commits focused and avoid unrelated refactors.

Application code is under `pkg/mixr`, supported build scripts are under
`pkg/mixr/scripts`, diagnostic commands are under `cmd`, and compatible
firmware is under `arduino`.

## Upstream attribution

mixr remains based on Omri Harel's deej project and retains its MIT license.
The inherited upstream contributing guide is preserved as
[deej-contributing.md](upstream/deej-contributing.md) for historical context;
its maintainer and contribution policies describe upstream deej, not mixr.
