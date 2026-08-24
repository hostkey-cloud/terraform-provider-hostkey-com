# Examples

**Apply (paid):** one file — [`basic/main.tf`](basic/main.tf)

```bash
cd examples/basic
export HOSTKEY_API_KEY="…"
cp terraform.tfvars.example terraform.tfvars   # set root_pass
terraform init && terraform apply
```

| Path | Purpose |
|------|---------|
| [basic](basic/) | **Simple apply** — one `main.tf` (`vm.pico` NL) |
| [provider](provider/) | Provider block only |
| [data-sources/catalog](data-sources/catalog/) | Read-only catalog |
| [resources/hostkey_server](resources/hostkey_server/) | Fuller server example |
| [resources/hostkey_ssh_key](resources/hostkey_ssh_key/) | SSH key |
| [resources/hostkey_dns_domain](resources/hostkey_dns_domain/) | DNS zone |
| [dev-terraform.rc](dev-terraform.rc) | Local `dev_overrides` |

Registry: [`hostkey-cloud/hostkey-com`](https://registry.terraform.io/providers/hostkey-cloud/hostkey-com/latest). RU: [`hostkey-cloud/hostkey-ru`](https://registry.terraform.io/providers/hostkey-cloud/hostkey-ru/latest). Old `hostkey-cloud/hostkey` is deprecated.
