---
page_title: "hostkey_server Resource - hostkey"
subcategory: ""
description: |-
  Orders and manages a Hostkey server via InvAPI.
---

# hostkey_server (Resource)

Orders a server with [`eq/order_instance`](https://hostkey.com/documentation/apidocs/eq/#order_instance), waits for deploy, manages hostname, tags, power, reboot and reinstall. Destroy calls `whmcs/request_cancellation`.

One resource covers the whole catalog. There is no separate GPU/VDS type:

| Prefix | Typical `server_type` | Example `preset_name` |
|--------|----------------------|------------------------|
| `vm.*` | Virtual Private Server | `vm.pico` |
| `vds.*` | Virtual Dedicated Server | `vds.ryzen-8` |
| `bm.*` | Instant Dedicated Server | `bm.v2-promo` |
| `gpu.*` | Dedicated GPU Server | `gpu.v2-a5000` |
| `vgpu.*` | VDS with GPU | `vgpu.v2-a4000` |

Always pair the preset with a traffic plan for that **preset id** (`instance_id` in [hostkey_traffic_plans](../data-sources/traffic_plans.md)). GPU dedic often uses unmetered-style plans; vGPU often uses dedic-style names (`1Gbps 50TB - FREE`). Confirm names and prices in the catalog.

Changing OS / software / `root_pass` / `ssh_key` (or `reinstall_trigger`) **reinstalls the same server id** — disk is wiped. Reinstall sends only install fields (`os_id`, software, `root_pass`, SSH key, RAID/LVM, scripts) — not location, traffic plan, extra IPv4, or VLAN. Changes to preset, location, traffic plan or billing period force **replace** (new order).

When you set only `os_name`, `soft_name`, or `traffic_plan_name`, the provider resolves the matching `*_id` at plan time (you do not need to set both). If an existing server needs reinstall, `terraform plan` still shows **update in-place**, but emits a **warning** that the disk will be wiped — treat that like a destructive change, not a tag update.

## Example Usage

### VPS

```hcl
resource "hostkey_server" "web" {
  preset_name       = "vm.pico"
  location_name     = "NL"
  os_name           = "Ubuntu 22.04"
  traffic_plan_name = "3 TB / 1 Gbps VM"
  deploy_period     = "monthly"
  root_pass         = var.root_pass
  power_state       = "on"
  cancellation_type = 1

  tags = {
    env = "prod"
  }

  timeouts {
    create = "90m"
    update = "90m"
    delete = "30m"
  }
}
```

### Dedicated

Dedicated presets use the same resource. Catalog names differ from the control-panel labels:

| Panel / docs | InvAPI `preset_name` / plan |
|--------------|-----------------------------|
| v2-promo | `bm.v2-promo` |
| 1Gbps unmetered (10000 ₽) | `1Gbps unmetered (10000 P)` (or id from catalog) |
| 1 IPv4 - Free | default (`ipv4_amount` omitted or `1`; provider never sends `ipv4_amount=1` to InvAPI) |
| IPv6 /64 block | `ipv6_block = true` — **only if the panel shows the checkbox** for this preset (NL/US; not all bm) |

Bare names like `1Gbps unmetered` can match **two** InvAPI rows (different prices). Prefer a price hint (`- FREE`, `(10000 P)`) or `traffic_plan_id`. Confirm with [`hostkey_traffic_plans`](../data-sources/traffic_plans.md) (`instance_id` = preset id).

Disk layout (panel «Disk configuration» / «Конфигурация дисков») maps to `disk_mirror` and `no_lvm`. Omit `root_size` for «100% от загрузочного диска» (`root_size` is a **percent** 1–100, not GB).

Set `disk_mirror` only when InvAPI `presets/list` shows **2+ disks** (`hdd` like `2x960` or description `/2x1TB`). One-disk presets (`hdd=1000`, `/1TB NVMe`) leave panel RAID type empty — **omit** `disk_mirror`; sending `hba` or RAID is not processed. `raid1`/`raid0` need two disks; `raid10` needs four.

```hcl
resource "hostkey_server" "dedic" {
  preset_name       = "bm.v2-promo"
  location_name     = "NL"
  os_name           = "Ubuntu 22.04"
  traffic_plan_name = "1Gbps unmetered (10000 P)"
  deploy_period     = "monthly"
  root_pass         = var.root_pass
  power_state       = "on"
  cancellation_type = 1

  # Catalog shows 1 disk — omit disk_mirror (panel RAID type is empty).
  no_lvm      = true  # classic partitions instead of LVM
  # root_size = 50    # percent of disk; omit = 100% (full boot disk)

  # Network (panel «Сетевые настройки»)
  ipv4_amount = 1       # default 1 IPv4
  # ipv6_block = true   # only when panel shows «IPv6 /64 block» for this preset (NL/US)

  timeouts {
    create = "90m"
    update = "90m"
    delete = "30m"
  }
}
```

List plans with [`hostkey_traffic_plans`](../data-sources/traffic_plans.md).

### Dedicated GPU (`gpu.*`) and vGPU (`vgpu.*`)

Same resource and attributes as VPS/dedicated — change **`preset_name`** and pick OS/traffic from the catalog for that preset (`instance_id`). Examples: `gpu.v2-a5000` + CUDA OS images; `vgpu.v2-a4000` + plans like `1Gbps 50TB - FREE`. Do **not** reuse VM traffic plan names.

## Argument Reference

### Required

- `location_name` (String) Data-center code (`NL`, `US`, `FI`, `DE`, `RU`, …). This provider always uses `invapi.hostkey.com` (not the `.ru` portal).
- `root_pass` (String, Sensitive) Root password (8–30 chars: upper, lower, digit, and one of `% - _ +`; no `@`/`#`). Change triggers reinstall.

### Optional

- `preset_name` / `preset_id` — catalog preset (name preferred): `vm.*`, `vds.*`, `bm.*`, `gpu.*`, `vgpu.*`. Change forces replace.
- `os_name` / `os_id` — OS. Setting `os_name` alone updates `os_id` at plan time. If both are set in HCL and disagree, plan fails with a clear conflict error (remove the stale `os_id` or align it). Change triggers reinstall.
- `soft_name` / `soft_id` — marketplace software. Setting `soft_name` alone updates `soft_id` at plan time. Change triggers reinstall.
- `traffic_plan_name` / `traffic_plan_id` — traffic plan for that preset (VM vs dedic names differ; dedic often needs `- FREE` / `(NNNN P)` hints or an id). Setting `traffic_plan_name` alone updates `traffic_plan_id` at plan time. Change forces replace.
- `hostname` — server hostname (rename via InvAPI). Optional+Computed: when unset at create, the provider generates a unique `tf-…` hostname for pending-order correlation. Terraform always sends `hostname` on `eq/order_instance` when set (and self-heals with `eq/rename_server` / `tags/add hostname` if InvAPI tags still mismatch after link). On **Read/refresh**, Terraform records the live InvAPI value from `eq/show` (usually the `hostname` tag) so drift shows on the next plan. On **Create/Update apply**, state keeps the planned hostname when InvAPI confirms it; if rename cannot make `eq/show` match, Update fails and keeps the prior live hostname (avoids an inconsistent-result loop — do not change HCL to the live name unless you accept it). Not the guest OS hostname — confirm with `hostname` / `hostnamectl` on the server if needed. Hostkey's readiness email Hostname field is filled at notify time from InvAPI metadata; if hostname was not applied yet, that email often shows the main IPv4 instead — rename after Create cannot rewrite an email already sent.
- `ssh_key` (String, Sensitive) — public key for deploy/reinstall. Change triggers reinstall. Marked sensitive so it is redacted from plan/apply output and logs.
- `post_install_script`, `own_os`, `root_size`, `disk_mirror`, `no_lvm`, `os_template` — install / disk options for bare metal (`bm.*`, `gpu.*`); changes trigger reinstall. `root_size` is percent of total disk (1–100), not GB. `post_install_script` max 32768 chars; `os_template` max 1024 chars.
- `deploy_period` — `hourly`, `monthly`, `quarterly`, `semi-annually`, `annually`. Forces replace.
- `deploy_notify` — email when deploy finishes (Hostname in that email may be the main IPv4 if InvAPI had not applied `hostname` yet; see `hostname` above).
- `ipv4_amount`, `ipv6_block`, `vlan`, `private_vlan`, `custom_domain`, `deploy_options` — order-time options (mostly force replace). `deploy_options` max 8192 chars; `deploy_options` and `os_template` are forwarded as-is — invalid values fail at order/reinstall time. `ipv6_block`: dedicated catalog `virtual=0` and location **NL or US**. InvAPI does not expose a per-preset IPv6 checkbox — set it only when the panel shows it.
- `ipv4_amount` is the **total** desired IPv4 count (1 = the default free address). The provider converts this to InvAPI's additive extra-address count and never sends `ipv4_amount=1` on the wire (sending `1` literally would bill one paid extra on top of the free default). Values `> 1` emit a plan warning because extras may be billed.
- `tags` (Map of String) — user tags only (Hostkey system tags are not synced back).
- `power_state` — `on` / `off`. Omit to leave power unmanaged.
- `power_off_hard` — use `eq/hard_off` when turning off.
- `reboot_trigger` — change the string to call `eq/reboot` once.
- `reinstall_trigger` — change the string to force reinstall with current OS/software.
- `cancellation_type` — `0` end of period, `1` immediate (when allowed). Used on destroy.
- `cancellation_reason` — reason for cancellation.
- `poll_interval_seconds` — deploy poll interval (default `15`).
- `timeouts` — `create` / `update` / `delete`.

### Read-Only

- `id` — InvAPI server id. After a Paid order, state may be `pending:<invoice>` until apply links the real id (plan shows an in-place update; apply waits, it does not re-order).
- `main_ipv4` — primary IPv4 after deploy.
- `status` — last known status.
- `invoice` — WHMCS invoice id after Paid.

## Import

```shell
terraform import hostkey_server.web 12345
```

Import by InvAPI server id (numeric only).

After import, Terraform state contains mainly **`id`**, **`main_ipv4`**, **`status`**, and power-related fields from `eq/show`. Catalog fields (`preset_name`, `os_name`, `traffic_plan_name`, computed ids) are **not** filled from the panel automatically — set them in HCL to document intent.

**First apply after import:** declaring `os_name`, `traffic_plan_name`, `root_pass`, etc. when those attributes were **empty in state** does **not** trigger reinstall. Reinstall runs when an install-time field **changes from a value already stored in state**, or when you set a non-empty **`reinstall_trigger`**.

To avoid surprise drift, either:

- keep HCL aligned with the live server and accept that some attributes stay unknown in state until a reinstall, or
- set **`reinstall_trigger`** once to align the OS/software with HCL (disk wipe).

## Notes

- `root_pass` is stored in Terraform state (sensitive).
- If create wait is interrupted (network/DNS), state stays `pending:<invoice>`. The next apply **waits for that invoice** via `eq/update_servers` `deploy_keys` / callback — it does not place a new order and does not adopt a sibling server. `terraform plan` is not a no-op while pending. Do **not** `terraform taint` / replace a pending resource: destroy of an unlinked pending only drops state and leaves the paid invoice in billing.
- If the Paid order is **cancelled or fails** in the Hostkey panel / InvAPI while Terraform is still waiting, apply stops early with Warning **«Deploy cancelled or failed»** (not «still in progress»). State remains `pending:<invoice>` so you can `terraform destroy` to drop tracking; the next apply fail-fasts with the same warning and does not re-order.
- While waiting, set `TF_LOG=INFO` to see InvAPI status hints on each poll. Terraform CLI itself always prints a fixed `Still creating... [elapsed]` line and cannot show custom status there.
- Pending create only resumes this resource's own `pending:<invoice>` — foreign Pending orders are never adopted.
- RAID / disk layout: [Hostkey RAID docs](https://hostkey.com/documentation/technical/exist_server_using/raid_create/).
