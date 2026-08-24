# Split: hostkey-ru + hostkey-com

Current tree is the **COM** fork (`terraform-provider-hostkey-com`).  
Sibling checkout: `../terraform-provider-hostkey` (becomes `terraform-provider-hostkey-ru`).

| | RU | COM |
|--|----|-----|
| GitHub | `hostkey-cloud/terraform-provider-hostkey-ru` | `hostkey-cloud/terraform-provider-hostkey-com` |
| Registry | `registry.terraform.io/hostkey-cloud/hostkey-ru` | `…/hostkey-com` |
| InvAPI | `https://invapi.hostkey.ru/` only | `https://invapi.hostkey.com/` only |
| Docs | Russian, hostkey.ru | English, hostkey.com |
| TypeName | `hostkey` (resources `hostkey_*`) | same |

## Breaking (v0.2.0)

- `source` is no longer `hostkey-cloud/hostkey` (deprecated).
- Provider attribute `region` removed; install the matching provider instead.
- `base_url` may override staging/localhost; the other portal’s host is rejected.

## Consumer migration

```hcl
terraform {
  required_providers {
    hostkey = {
      source  = "hostkey-cloud/hostkey-com" # or hostkey-ru
      version = "~> 0.2"
    }
  }
}
provider "hostkey" {
  api_key = var.hostkey_api_key
}
```

```bash
terraform state replace-provider \
  'registry.terraform.io/hostkey-cloud/hostkey' \
  'registry.terraform.io/hostkey-cloud/hostkey-com'
```

## Fork-specific code

- [`internal/invapi/portal.go`](internal/invapi/portal.go) — default URL, allowed TLD, sibling provider error
- [`main.go`](main.go) `Address`
- `go.mod` module path
- Makefile binary / User-Agent
- README + `docs/` language and links

## GitHub / Registry (manual)

1. Push this tree to `terraform-provider-hostkey-com` (new origin; separate GoReleaser GPG secrets).
2. Register `hostkey-cloud/hostkey-com` on Terraform Registry.
3. Stop publishing tags that release to old `hostkey-cloud/hostkey`.
4. Tag **v0.2.0** after Registry is pointed at this GitHub name.

Hard ban: never ship server id **56909**.
