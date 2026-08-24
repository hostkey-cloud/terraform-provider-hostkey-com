# Hostkey | Terraform Provider (COM)

[![Terraform Registry](https://img.shields.io/badge/registry-hostkey--cloud%2Fhostkey--com-623CE4)](https://registry.terraform.io/providers/hostkey-cloud/hostkey-com/latest)

Terraform provider for [Hostkey](https://hostkey.com/) (**.com** portal, InvAPI `invapi.hostkey.com`): VPS, dedicated, GPU, and DNS.

Russian / `.ru` portal: [`terraform-provider-hostkey-ru`](https://github.com/hostkey-cloud/terraform-provider-hostkey-ru) (`hostkey-cloud/hostkey-ru`).

> **Migrating from `hostkey-cloud/hostkey`:** set `source` to `hostkey-cloud/hostkey-com`, drop `region`, then  
> `terraform state replace-provider 'registry.terraform.io/hostkey-cloud/hostkey' 'registry.terraform.io/hostkey-cloud/hostkey-com'`.

## Documentation

Full attribute reference: [`docs/`](docs/) ([Terraform Registry](https://registry.terraform.io/providers/hostkey-cloud/hostkey-com/latest/docs)). Examples: [`examples/`](examples/). Split checklist: [`SPLIT.md`](SPLIT.md).

### Resources

| Resource | Purpose |
|----------|---------|
| [`hostkey_server`](docs/resources/server.md) | Order and manage a server (VPS, dedic, GPU, vGPU) |
| [`hostkey_server_ip`](docs/resources/server_ip.md) | Extra IPv4 on a server |
| [`hostkey_ssh_key`](docs/resources/ssh_key.md) | SSH key in the InvAPI account store |
| [`hostkey_dns_domain`](docs/resources/dns_domain.md) | DNS zone (PowerDNS) |
| [`hostkey_dns_record`](docs/resources/dns_record.md) | Record in a DNS zone |

### Data sources

| Data source | Purpose |
|-------------|---------|
| [`hostkey_presets`](docs/data-sources/presets.md) | Preset list (`presets/list`) |
| [`hostkey_preset`](docs/data-sources/preset.md) | One preset by id or name |
| [`hostkey_oses`](docs/data-sources/oses.md) | OS images for a preset or server |
| [`hostkey_traffic_plans`](docs/data-sources/traffic_plans.md) | Traffic plans for a location / preset |
| [`hostkey_software`](docs/data-sources/software.md) | Marketplace software for a preset |
| [`hostkey_ssh_keys`](docs/data-sources/ssh_keys.md) | Account SSH keys |
| [`hostkey_dns_domains`](docs/data-sources/dns_domains.md) | Account DNS zones |

One `hostkey_server` resource covers the catalog: `vm.*`, `vds.*`, `bm.*`, `gpu.*`, `vgpu.*`. Dedicated / GPU: [docs/resources/server.md](docs/resources/server.md).

## Requirements

* [Terraform](https://developer.hashicorp.com/terraform/tutorials/aws-get-started/install-cli) **>= 1.0**
* Account-wide InvAPI API key (`Any`): [documentation](https://hostkey.com/documentation/account/api_key_account/)
* Ordering a server is **paid**; deploy can take up to ~90 minutes

## Quick start

### 1. Configuration

Ready one-file example: [`examples/basic/main.tf`](examples/basic/main.tf). Or copy:

```hcl
terraform {
  required_providers {
    hostkey = {
      source  = "hostkey-cloud/hostkey-com"
      version = "~> 0.2"
    }
  }
  required_version = ">= 1.0"
}

provider "hostkey" {}

variable "root_pass" {
  type      = string
  sensitive = true
}

resource "hostkey_server" "web" {
  preset_name       = "vm.pico"
  location_name     = "NL"
  os_name           = "Ubuntu 22.04"
  traffic_plan_name = "3 TB / 1 Gbps VM"
  deploy_period     = "monthly"
  root_pass         = var.root_pass
  power_state       = "on"
  cancellation_type = 1

  timeouts {
    create = "90m"
    delete = "30m"
  }
}

output "server_id"  { value = hostkey_server.web.id }
output "main_ipv4" { value = hostkey_server.web.main_ipv4 }
```

Copy [`examples/basic/terraform.tfvars.example`](examples/basic/terraform.tfvars.example) → `terraform.tfvars` (gitignored, **do not commit**):

```hcl
root_pass = "StrongPass1%"
```

### 2. API key

InvAPI → **Username → API keys**: [hostkey.com](https://hostkey.com/documentation/account/api_key_account/) · [invapi.hostkey.com](https://invapi.hostkey.com).

**Option 1 — environment (recommended):**

```bash
export HOSTKEY_API_KEY="your-key"
```

**Option 2 — provider block:**

```hcl
variable "hostkey_api_key" {
  type      = string
  sensitive = true
}

provider "hostkey" {
  api_key = var.hostkey_api_key
}
```

Env aliases: `HOSTKEY_API_TOKEN`. URL override (staging / localhost): `HOSTKEY_BASE_URL` / `HOSTKEY_API_URL`. `.ru` hosts are rejected — use `hostkey-cloud/hostkey-ru`.

### 3. Init, validate, plan, apply

```bash
terraform init
terraform validate
terraform plan
terraform apply
```

Orders are **paid**. Create is async (default timeout 90m).

### 4. Destroy

```bash
terraform destroy
```

Calls `whmcs/request_cancellation` with `cancellation_type` / `cancellation_reason`.

## InvAPI notes

* This provider always uses **`invapi.hostkey.com`**. Resource **`location_name`** is the data center (`NL`, `US`, `RU`, …), not the portal.
* Provider **`region` was removed** (v0.2). For `.ru` use `hostkey-cloud/hostkey-ru`.
* **`preset_name` / `os_name` / `traffic_plan_name`** must match InvAPI exactly (`bm.v2-promo`, not `v2-promo`).
* Before ordering: `data.hostkey_presets` + `data.hostkey_traffic_plans` with **`instance_id`** = preset id.
* Dedicated often has **two plans with the same `name` and different `price`** — use panel hints (`- FREE`, `(10000 P)`) or `traffic_plan_id`.
* **`disk_mirror`** only if `presets/list` shows **2+ disks**; omit on one-disk presets (including `bm.v2-promo`).
* **`hostname`** in state comes from InvAPI `eq/show` (usually the `hostname` tag), not the guest OS.

Local build: `go install` + [dev_overrides](examples/dev-terraform.rc) — [CONTRIBUTING.md](CONTRIBUTING.md).

## Import

```bash
terraform import hostkey_server.web 12345
```

See [Registry: hostkey_server → Import](https://registry.terraform.io/providers/hostkey-cloud/hostkey-com/latest/docs/resources/server#import).

## Troubleshooting

Empty account (`NO_APPROPRIATE_SERVERS`): InvAPI will not issue a session with **zero servers**. Order the first server in the panel, then use the provider. More: [Registry: Troubleshooting](https://registry.terraform.io/providers/hostkey-cloud/hostkey-com/latest/docs#troubleshooting).

## Development

* [CONTRIBUTING.md](CONTRIBUTING.md)
* [SECURITY.md](SECURITY.md)
* [CHANGELOG.md](CHANGELOG.md)
* License [MPL-2.0](LICENSE)
