---
page_title: "hostkey_servers Data Source - hostkey"
subcategory: ""
description: |-
  Lists servers in the Hostkey account.
---

# hostkey_servers (Data Source)

Lists servers available to the account (`eq/list`), with details from `eq/show` for each server.

```hcl
data "hostkey_servers" "all" {}

output "servers" {
  value = data.hostkey_servers.all.servers
}

# find one server by hostname, e.g. to import it
data "hostkey_servers" "web" {
  hostname = "web-1"
}
```

## Argument Reference

### Optional

- `hostname` (String) Exact hostname filter (as returned by `eq/show`).

### Read-Only

- `ids` (List of Number) Server IDs, sorted ascending.
- `servers` — list of objects with `id`, `hostname`, `main_ipv4`, `status`, `location`.

`hostname`, `status` and `location` are read best-effort from `eq/show`; an empty string means the value was not found in the response. One `eq/show` call is made per server.
