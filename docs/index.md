---
page_title: "hostkey Provider"
description: |-
  Terraform provider for Hostkey InvAPI (servers, SSH keys, IPs, DNS) on the .com portal.
---

# Hostkey Provider (COM)

Manage Hostkey infrastructure via [InvAPI](https://hostkey.com/documentation/apidocs/api_index/) (`invapi.hostkey.com`).  
Account API keys: InvAPI → **Username → API keys** ([docs](https://hostkey.com/documentation/account/api_key_account/)).

`.ru` portal: provider [`hostkey-cloud/hostkey-ru`](https://registry.terraform.io/providers/hostkey-cloud/hostkey-ru/latest).

Quick start: [GitHub README](https://github.com/hostkey-cloud/terraform-provider-hostkey-com/blob/main/README.md).

## Migrating from `hostkey-cloud/hostkey`

1. Set `source` to `hostkey-cloud/hostkey-com`, version `~> 0.2`.
2. Remove the `region` attribute.
3. `terraform state replace-provider 'registry.terraform.io/hostkey-cloud/hostkey' 'registry.terraform.io/hostkey-cloud/hostkey-com'`

## Example

```hcl
terraform {
  required_providers {
    hostkey = {
      source  = "hostkey-cloud/hostkey-com"
      version = "~> 0.2"
    }
  }
}

provider "hostkey" {
  # api_key from HOSTKEY_API_KEY / HOSTKEY_API_TOKEN, or set explicitly
}
```

## Schema

### Optional

- `api_key` (String, Sensitive) Account InvAPI API key. Env: `HOSTKEY_API_KEY` or `HOSTKEY_API_TOKEN`.
- `base_url` (String) InvAPI URL override. Default `https://invapi.hostkey.com/`. HTTP only for `localhost`. `.ru` hosts are rejected. Env: `HOSTKEY_BASE_URL` or `HOSTKEY_API_URL`.
- `token_ttl` (Number) Session token TTL in seconds for `auth/login` (default `3600`).
- `http_timeout` (Number) HTTP client timeout in seconds (default `60`).
- `max_retries` (Number) Max attempts for retryable InvAPI HTTP failures (default `3`).

## Troubleshooting

| Error / symptom | What to do |
|-----------------|------------|
| `InvAPI account has no servers` / `NO_APPROPRIATE_SERVERS` | InvAPI will not issue a session on an **empty** account (zero servers) — not a wrong API key. Order the first server in the Hostkey panel, then re-run Terraform. Use an account-wide (`Any`) key. |
| Other `auth/login` failures | Account-wide key (`Any`); this provider only talks to `invapi.hostkey.com` |
| `InvAPI host … belongs to the other Hostkey portal` | Use [`hostkey-cloud/hostkey-ru`](https://registry.terraform.io/providers/hostkey-cloud/hostkey-ru/latest) |
| `Catalog verification failed` | Run `terraform plan` with a configured provider; confirm preset/OS/traffic ids via data sources |
| Ambiguous `traffic_plan_name` | List plans with [hostkey_traffic_plans](data-sources/traffic_plans.md) and `instance_id`; use `(10000 P)` / `- FREE` hints or `traffic_plan_id` |
| `pending:<invoice>` id | Deploy still running after a Paid order. `terraform plan` shows an in-place update; `apply` waits for **this invoice**. Live status is in the Hostkey panel |
