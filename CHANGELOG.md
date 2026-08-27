# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.1] - 2026-08-27

### Fixed

- `hostkey_server` reinstall: send `hostname` and `location_name` on `eq/order_instance` (InvAPI needs location for OS compatibility; hostname was omitted so reinstall could never apply the planned name even at InvAPI/tags level during wipe). Guest OS hostname remains a separate InvAPI/OpenStack channel — live reinstall still showed OS names like server id / `*.example.fi` / preset-like `vm-pico` while tags matched HCL (`fence_openstack_in` receives `hostname` but `cloud_init_script` is null unless set separately).
- `eq_callback/check`: treat InvAPI `result=Not ready` as in-progress (not an API error) so reinstall/create callback waits can poll instead of failing on the first check.
- Private state keys (`order_callback`, `reinstall_callback`, `order_terminal_error`): store JSON-encoded strings so Plugin Framework accepts them (raw callback hex previously failed with «must be valid JSON»).

### Changed

- `hostkey_server`: on **Unpaid** invoice, Create/Update now **waits for payment in the same apply** (polls `whmcs/get_invoices` within create timeout), then continues deploy link — no second apply required if you pay while `Still creating...`. Soft Warning **Waiting for invoice payment** only if still unpaid when create timeout expires (`pending:<invoice>` kept; re-apply resumes, no re-order).

## [0.2.0] - 2026-08-24

- Sibling RU provider namespace is **`hostkey-cloud-ru/hostkey-ru`** (GitHub `hostkey-cloud-ru/terraform-provider-hostkey-ru`). This COM provider remains `hostkey-cloud/hostkey-com`.

First release of the **COM-only** provider. GoReleaser tag: **`v0.2.0`**. Registry: [`hostkey-cloud/hostkey-com`](https://registry.terraform.io/providers/hostkey-cloud/hostkey-com/latest). Sibling: [`hostkey-cloud-ru/hostkey-ru`](https://registry.terraform.io/providers/hostkey-cloud-ru/hostkey-ru/latest). Split checklist: [SPLIT.md](SPLIT.md).

### Breaking

- **New Registry source.** Install `hostkey-cloud/hostkey-com` (this repo). The combined source `hostkey-cloud/hostkey` is deprecated and will not receive 0.2.x.
- **Provider `region` removed.** Each provider talks to one InvAPI portal. This one is always `https://invapi.hostkey.com/`. For `invapi.hostkey.ru` use `hostkey-cloud-ru/hostkey-ru`.
- **`base_url`** may still override staging/`localhost` on `*.hostkey.com`. Hosts on `*.hostkey.ru` are rejected with a pointer to `hostkey-ru`.
- **Go module** is `github.com/hostkey-cloud/terraform-provider-hostkey-com`. Binary / User-Agent: `terraform-provider-hostkey-com`.

### Fixed

- `hostkey_server`: when `order_instance` returns an **Unpaid** invoice (auto-pay off or insufficient credit), Create no longer polls until create-timeout with only `Still creating...`. Apply exits immediately with Warning **Waiting for invoice payment**, keeps `pending:<invoice>`, and the next apply resumes after payment (no re-order). Pending wait/Read also detect unpaid invoices via `whmcs/get_invoices`.

### Changed

- Registry docs (`docs/`) and README are English / [hostkey.com](https://hostkey.com).
- `examples/basic/main.tf` is a single-file paid apply (`vm.pico` NL): empty `provider "hostkey" {}`, API key from `HOSTKEY_API_KEY`, only `root_pass` in tfvars.

### Added

- [`internal/invapi/portal.go`](internal/invapi/portal.go): portal default URL, TLD allowlist, sibling-provider error strings.

## [0.1.9] - 2026-08-21

### Fixed

- Provider Configure: InvAPI `auth/login` response `NO_APPROPRIATE_SERVERS` / «No appropriate servers found» (empty account with zero servers) is no longer a generic «authentication failed» — the error explains that InvAPI refuses a session on an empty account (not a wrong API key) and that the first server must be ordered in the panel first.

- `hostkey_server`: when a Paid pending deploy is **cancelled or fails** in the panel / InvAPI (callback `result=Error` / failed/cancelled, scope/context error tokens, or async callback key purged), Create/Update/Read stop waiting immediately with Warning **«Deploy cancelled or failed»** instead of polling until create-timeout and saying «Deploy still in progress». State stays `pending:<invoice>` (destroy drops tracking only; no re-order). The terminal reason is stored in private state so the next apply fail-fasts with the same Warning.

- `hostkey_server`: when `preset_name` / `preset_id` is missing from `presets/list` for the configured `location_name`, the error now hints the catalog `locations` string if the preset exists globally (e.g. `bm.v1-str-8t` is NL,FI-only — unfiltered `presets.php` still lists it). Preset `description` (hardware) remains unrelated to `traffic_plan_name`.

- `hostkey_server`: hostname Create self-heal no longer treats a missing `eq/show` hostname as a match against config (empty `ShowHostname` previously left the planned value in state and skipped `eq/rename_server`). Read warns when live hostname cannot be determined. Create/Update emit a **Verify guest OS hostname** warning: InvAPI only exposes hostname via `eq/show` tags/`server_data`, not the guest OS — tags can say `web-01` while the OS still shows a preset-like name (`vm-v2-pico`). When InvAPI live hostname ≠ config, **Read** still records the live value so the next plan shows a change (Create/Update apply must keep planned hostname — next bullet).

- `hostkey_server`: Update hostname after import no longer hits **Provider produced inconsistent result after apply** (plan `web-01`, apply wrote live `nl-vmpico` from `eq/show`). Create/Update apply keep the planned hostname in state; Read/refresh still records live InvAPI drift. After a planned rename, the provider calls `eq/rename_server`, then `tags/add` for the `hostname` tag when `ShowHostname` still lags, and polls briefly — if InvAPI never reflects the new name, Update returns a clear Error and keeps the prior live hostname so the next plan still shows the change (no crash loop; no need to edit HCL to the live name).

- `hostkey_server`: Create warns when InvAPI `eq/show` hostname is still empty or an IP at link time — Hostkey readiness emails often put that value (or the main IPv4) in the Hostname field, and `eq/rename_server` self-heal cannot rewrite an email already sent. `hostname` continues to be sent on `order_instance` (trimmed); docs note email vs InvAPI tags vs guest OS.

## [0.1.8] - 2026-08-21

### Fixed

- `hostkey_server`: concurrent Creates in the same process can no longer all link to the same newcomer server id when `eq/show` hostname still lags. Pending resolution now uses a process-local claim registry (keyed by invoice) so only one waiter may adopt a given id; other waiters keep polling for a different newcomer or hostname/callback match. Serial single-newcomer linking is unchanged.
- InvAPI: `ShowHostname` / pending correlation also read hostname from `eq/show` top-level `tags[]` (`tag=hostname` or `server_name`) — InvAPI often omits hostname from `server_data`, which previously broke hostname match and rename self-heal.
- `hostkey_server`: when Create has a requested/default hostname, pending link no longer claims the first single newcomer with empty/mismatched hostname; it waits for a tags/`server_data` hostname match. Blind single-newcomer accept remains only when `wantHostname` is empty (avoids FI↔RU cross-link under parallel apply).
- `hostkey_server`: a sole unclaimed newcomer whose live hostname is still InvAPI’s default `hostkey{id}` is treated as “hostname not applied yet” and may be claimed when `wantHostname` is set; Create then self-heals via `eq/rename_server`. Empty live hostname still waits (parallel safety).


## [0.1.7] - 2026-08-20

### Removed

- `hostkey_server`: removed the `extra_order_params` attribute entirely. It was a decoy: any key set on it always failed plan validation, so it could never forward anything — it only added confusion and an unused `RequiresReplace` plan modifier. All real order-time fields are the existing typed attributes.

### Changed

- InvAPI client: retries no longer blindly replay `net/add_ipv4`, `pdns/add_domain`, `pdns/add_dns`, `ssh_keys/add`, or `tags/add` on timeout/5xx, matching the existing `eq/order_instance` protection — each creates a resource with no server-side idempotency key, so a lost response after a successful write could otherwise duplicate an IP, DNS record, SSH key, or tag.
- InvAPI: `showHostname` only trusts a bare `name` key at the top level of `server_data`; `hostname`/`server_name` remain trusted at any depth (avoids mistaking nested catalog `name` fields for the server hostname during pending correlation).
- Build: require Go **1.26.6** (stdlib fixes for `net/url`, `crypto/tls`, `encoding/asn1`, `net/http` reported by govulncheck).
- `hostkey_server`: static, zero-network config checks (own_os/os_template/deploy_options/ipv4_amount/vlan/private_vlan warnings, `power_off_hard` requires `power_state=off`, bare-metal option checks, tag length limits) moved from the `ModifyPlan` hook into a new `ValidateConfig` implementation, so they now surface on a bare `terraform validate` without provider credentials. The create-only "preset/OS/traffic plan required" checks stay in `ModifyPlan` since they need `isCreate`, which `ValidateConfig` cannot determine.
- `hostkey_server`: `ssh_key` is now marked `Sensitive` so it is redacted from plan/apply output and logs.
- InvAPI client: catalog list calls (`presets/list`, `os/list`, `traffic_plans/list`, `software/list`) are now cached in-process for 30s per unique request, cutting redundant lookups during `ModifyPlan` + `Create`/`Update` for the same operation.
- InvAPI client: retry backoff between attempts is now exponential with full jitter (capped at 8s) instead of a fixed linear delay, and a `Retry-After` response header (capped at 30s) is honored when present.
- InvAPI client: HTTP response bodies are now read through a hard 8 MiB cap instead of unbounded, so a misbehaving/compromised upstream response can no longer exhaust memory.
- `root_size` is documented and validated as a **percentage** of total disk (1–100), matching InvAPI — not GB. The earlier GB-vs-`hdd` capacity check was incorrect and was removed.
- Pending wait polls log InvAPI status hints via `tflog` (`TF_LOG=INFO`) — Terraform CLI still hardcodes the `Still creating...` line and cannot show custom status there.
- Create with `ssh_key` emits a warning to verify key login (InvAPI does not expose authorized_keys confirmation).

### Added

- CI: added `gosec` and `bodyclose` to golangci-lint, added a `govulncheck` dependency-vulnerability step, and pinned `actions/checkout`/`actions/setup-go`/`hashicorp/setup-terraform` to commit SHAs.
- Tests: added an import-then-replan empty-plan assertion to `TestAccServer_basic` (import-safe replace regression guard), plus new `TestAccServerIP_basic` and `TestAccDNSRecord_basic` acceptance tests covering `hostkey_server_ip` and `hostkey_dns_record`, which previously had none.

### Fixed

- `hostkey_server`: `ipv4_amount` is treated as the **total** desired IPv4 count (1 = default free address, matching docs). InvAPI's `order_instance` parameter is additive on top of that default — previously sending `ipv4_amount=1` billed one paid extra. The provider now omits the wire param for 0/1 and forwards `ipv4_amount - 1` only when requesting more than the default.
- `hostkey_server`: `main_ipv4` and `status` use `UseStateForUnknown`, so a no-op plan no longer perpetually shows `1 to change` with only those fields as `(known after apply)`.
- `hostkey_server`: when both `os_name`/`soft_name`/`traffic_plan_name` and an explicit conflicting `*_id` are set in HCL, plan fails with a clear catalog conflict message instead of `Provider produced invalid plan`.
- `hostkey_server`: pending deploy timeout on Update now emits a **warning** (state kept as `pending:<invoice>`), matching Create — avoids failed apply / replace pressure that could drop a paid order from state on destroy.
- `hostkey_server`: after create, if InvAPI did not apply `hostname`, provider attempts `eq/rename_server` and warns on residual drift; Read records live hostname from `eq/show` when available.
- `hostkey_server`: Read removes the resource from state when InvAPI reports the server as not found (cancelled/deleted outside Terraform), instead of failing every plan.
- `hostkey_server`: plan warns when `location_name` is unknown so catalog checks are deferred to apply (instead of a silent skip).
- `hostkey_server`: import-safe RequiresReplace — null→known after import no longer forces replace.
- `hostkey_server`: Create generates a unique default `hostname` when unset and serializes snapshot+`order_instance` to reduce pending mis-correlation under parallelism.
- `hostkey_server`: pending link accepts a **single** new server id from `eq/update_servers` / `eq/list` even when InvAPI `eq/show` has not published hostname yet (hostname is self-healed via `eq/rename_server` after link). Previously a requested hostname blocked linking forever when `deploy_keys`/callback were empty.
- InvAPI: `APIError` messages redact secrets in Message/Result at construct and in `Error()`.
- InvAPI: treat `Invalid hash` (and similar) as auth failure — invalidate session and re-login once; pending link falls through from a failed `eq/update_servers` to `eq/list` / single-newcomer when callback is empty (avoids stuck `pending:<invoice>`).
- `hostkey_dns_record`: `Read` now refreshes `ttl`/`priority` from the live zone when those fields are already tracked in state, so out-of-band edits are surfaced as drift on the next plan instead of being silently masked.

## [0.1.6] - 2026-08-19

### Fixed

- `hostkey_server` Create: when `eq/list` / `eq/update_servers` exposes exactly one new server id after the pre-order snapshot, apply now links it immediately and does not wait for `eq/show` hostname fields to appear. This fixes a case where the server was already created/active but Terraform kept printing `Still creating...`.

### Added

- Example: `examples/resources/hostkey_server_pending_resume/` for reproducing and verifying pending-create resume/link behavior during local testing.

## [0.1.5] - 2026-08-19

### Fixed

- `hostkey_server` Create: pending deploy no longer waits forever when the server is already active but callback data is incomplete or missing. Apply can now finish via a safe `eq/list` fallback with hostname disambiguation, without blindly adopting unrelated servers.

## [0.1.4] - 2026-08-19

### Fixed

- `hostkey_server`: interrupted create (`pending:<invoice>`) no longer plans as no-op. Next apply waits for **this** invoice/callback (`deploy_keys[invoice]`), not the first new `eq/list` id. Transient `eq/update_servers` errors are retried until timeout. Empty/failed pre-order `eq/list` refuses `order_instance`.
- InvAPI client: `eq/order_instance` is never HTTP-retried (avoids duplicate paid orders after timeout/5xx).
- `hostkey_dns_record` destroy: `pdns/delete_dns` includes record **content** (and MX priority when set) so destroy removes one row, not every record of that type on the name.
- `hostkey_ssh_key` Read: drop state only when the key is missing; keep state on InvAPI/network errors (avoids recreating the account default key after a timeout).
- InvAPI HTTP client: follow redirects only on the same origin (307/308 no longer replay `token`/`root_pass` to a third host). `base_url` must be HTTPS except localhost. Login JSON `invapi` is applied only for Hostkey hosts and never downgrades TLS.
- Diagnostics: login errors no longer dump response bodies; `token`/`key`/`password`/`root_pass` are redacted in truncated HTTP bodies.
- `hostkey_server` reinstall: send only install fields (`os_id`, software, `root_pass`, SSH, RAID/LVM, scripts) — not location, traffic plan, extra IPv4, VLAN, or IPv6.
- `hostkey_server` plan: changing only `os_name` / `soft_name` / `traffic_plan_name` now syncs the matching `*_id` (computed ids from state no longer block catalog verify).
- `hostkey_server` reinstall: if `WaitForCallback` fails, the next apply resumes waiting and does not start a second reinstall (prevents double disk wipes).
- `hostkey_server`: added bounded length validators for `os_template`, `deploy_options`, `post_install_script` to avoid unbounded client-side payloads.
- Release workflow: pin GitHub Actions to commit SHAs; `persist-credentials: false` on checkout.
- CI: acc helpers in `provider_test.go` are behind `//go:build acceptance` (same as `acc_test.go`) so `unused` lint does not fail default `golangci-lint`

### Added

- `hostkey_server` plan: warning when install-time fields trigger reinstall (disk wipe) even though Terraform shows update in-place.
- `hostkey_server` plan: add an `os_name`-scoped attribute warning for disk wipe on OS change.
- `hostkey_server` plan: warn when `ipv4_amount > 1` because extra IPv4 addresses may be billed.
- `hostkey_ssh_key` plan: warn when `default = true` because future server deploys may use the account default key automatically.
- README RU/EN and Registry troubleshooting: install via Yandex Cloud public Terraform provider mirror when `registry.terraform.io` is blocked (`terraform-mirror.yandexcloud.net`; no Yandex Cloud account)

## [0.1.3] - 2026-08-17

### Fixed

- `hostkey_server` Create: reject server ids from order callback / `WaitForNewServerID` that were already in `eq/list` before `order_instance` (`acceptNewServerID` + known-id check on deploy_keys path)
- Catalog verify: OS/traffic/software must be **active** in InvAPI lists (no fallback to inactive rows); cross-check `*_name` vs `*_id` when both are set
- `ModifyPlan`: error when the provider is not configured (`api_key` / env missing) instead of skipping catalog validation
- Import / first apply: declaring install fields (`os_name`, `traffic_plan_name`, `root_pass`, …) when state had them empty no longer triggers unintended reinstall
- Remove dead `OrderInstanceRequest.Extra` from InvAPI client (order fields are typed; `extra_order_params` stays closed in schema)

### Changed

- README RU/EN: Timeweb-style quick start; dedicated/GPU details in `docs/resources/server.md`; import/troubleshooting link to Registry docs
- `docs/index.md`: provider schema and troubleshooting (no duplicate resource index — Registry sidebar)
- `docs/resources/server.md`: import notes, dedicated example, compact GPU/vGPU section (removed duplicate HCL blocks)
- `docs/data-sources/presets.md`, `traffic_plans.md`: disk-count hints; traffic example without hardcoded preset id
- `examples/README.md`: Registry is published; local dev path clarified
- Plan warnings for `os_template` and `deploy_options`; order response no longer logged with raw InvAPI body
- Acceptance tests behind `//go:build acceptance` — `go test ./...` skips paid deploy even when `TF_ACC=1` is set (`make testacc` / `-tags=acceptance`)
- Sanitize public ids in tests and docs; `.gitignore` for `SECURITY_AUDIT*.md` and `acc-*.log`; `CONTRIBUTING.md`

## [0.1.2] - 2026-08-14

### Fixed

- CI: `gofmt` on `validators_common.go`; unused assignment in `catalog_resolve_test.go`

## [0.1.1] - 2026-08-14

### Added

- `hostkey_server`: bare-metal disk options `disk_mirror` (hba/raid0/raid1/raid10), `no_lvm`, and network `ipv6_block` (NL/US) with validation and bm.* docs example
- Schema and plan validation across provider/resources/data sources: InvAPI URLs, location codes, IPv4/IPv6, DNS zones/records, SSH keys, root password, server IDs, import IDs, cross-field server/DNS checks
- Plan: `disk_mirror` is checked against InvAPI `presets/list` disk count (`hdd`/`description`); 1 disk → omit the field; RAID10 needs 4+ disks. `extra_order_params` is closed (any key rejected, not forwarded)
- Catalog hardening: plan/apply re-check preset/OS/traffic/software against InvAPI lists; exact catalog names only; duplicate same-price traffic names require `traffic_plan_id` or a price hint

### Fixed

- Resolve dedicated traffic plans when InvAPI returns duplicate names: accept panel-style hints (`- FREE`, `(10000 P)`); ambiguous same-price rows require `traffic_plan_id`
- Documentation links: account API keys (`account/api_key_account`), RU README on `hostkey.ru`, EN/Registry on `hostkey.com`

### Changed

- Docs: dedicated uses `bm.v2-promo`; traffic plan examples aligned with InvAPI (`1Gbps 50TB - FREE`, `1Gbps unmetered (10000 P)`); GPU (`gpu.*`) and vGPU (`vgpu.*`) examples on `hostkey_server`
- Documentation reorganized for public release (README RU/EN, Registry `docs/`, consolidated contributor docs)

## [0.1.0] - TBD

First public release (tag `v0.1.0`) after GitHub + Registry setup.

### Added

- Provider `hostkey` for Hostkey InvAPI (`RU` / `COM`)
- Resources: `hostkey_server`, `hostkey_server_ip`, `hostkey_ssh_key`, `hostkey_dns_domain`, `hostkey_dns_record`
- Data sources: presets, preset, oses, traffic_plans, software, ssh_keys, dns_domains
- Server: catalog name → id resolve, tags, hostname, power, reboot, reinstall, cancellation
- Provider knobs: `http_timeout`, `max_retries`; env aliases `HOSTKEY_API_TOKEN`, `HOSTKEY_API_URL`
- GoReleaser + GitHub Actions (CI / Release)
