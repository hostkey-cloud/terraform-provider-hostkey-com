# Contributing

Thanks for helping improve the Hostkey Terraform provider.

## Development

Requirements: Go **1.26+** (see `go.mod`), Terraform CLI **>= 1.0** for local plan/apply.

```bash
go test ./...
go install -ldflags "-X main.version=dev"
```

For local Terraform against your build, copy [examples/dev-terraform.rc](examples/dev-terraform.rc) to the Terraform CLI config (`~/.terraformrc` or `%APPDATA%\terraform.d\terraform.rc`) and point it at `$(go env GOPATH)/bin`.

Useful Make targets: `make test`, `make build`, `make install`, `make lint`, `make testacc`.

## Layout

| Path | Role |
|------|------|
| `internal/provider` | Framework resources and data sources |
| `internal/invapi` | InvAPI HTTP client and auth |
| `cmd/smoke` | Auth smoke check (read-only InvAPI) |
| `docs/` | Registry documentation |
| `examples/` | Sample configurations |

## Tests

- Unit: `go test ./...` or `make test` (does **not** run acceptance tests).
- Smoke: `HOSTKEY_API_KEY=… go run ./cmd/smoke` (read-only InvAPI, this fork → `invapi.hostkey.com`).
- Acceptance (billed, production InvAPI):

```bash
export TF_ACC=1
export HOSTKEY_API_KEY=…
go test -tags=acceptance ./internal/provider -v -timeout 180m -run TestAcc
# or: make testacc
```

DNS acceptance needs `HOSTKEY_ACC_DNS_DOMAIN`. Do not point tests at servers you must not destroy.


## Release

Update [CHANGELOG.md](CHANGELOG.md) first. Tag `v*` (e.g. `v0.2.0`) and push the tag. [`.github/workflows/release.yml`](.github/workflows/release.yml) runs GoReleaser (Actions secrets `GPG_PRIVATE_KEY`, `PASSPHRASE`). Registry source is `hostkey-cloud/hostkey-com`. Do not commit API keys or Terraform state.

## Pull requests

- Keep diffs focused; match existing style.
- Update `docs/` and README when changing user-facing schema.
- Do not commit secrets, state, or personal account IDs.
- Do not commit `SECURITY_AUDIT.md`, GPG key files (`*.asc`), acceptance logs (`acc-*.log`), or local scripts under `scripts/` (gitignored).
- Acceptance tests must not hardcode production server ids you must not mutate (e.g. personal dev servers).
- CI must pass (`gofmt`, `vet`, lint, unit tests).

## Scope

Features that map poorly to declarative Terraform (S3, backups, ISO as resources, one-shot console/IPMI) should be discussed before implementation.

## License

Contributions are under [MPL-2.0](LICENSE).
