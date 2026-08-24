---
page_title: "hostkey_traffic_plans Data Source - hostkey"
subcategory: ""
description: |-
  Lists Hostkey traffic plans.
---

# hostkey_traffic_plans (Data Source)

Lists traffic plans ([`traffic_plans/list`](https://hostkey.com/documentation/apidocs/traffic_plans/#traffic_planslist)). InvAPI often requires `location`. The provider requests this list **without** a customer session token (sending a token frequently breaks the call).

VPS and dedicated presets use **different** plan names. Examples (exact strings change — always check the list):

| Kind | Example name in HCL |
|------|---------------------|
| VPS | `3 TB / 1 Gbps VM` |
| Dedicated | `1Gbps 50TB - FREE` (panel) → InvAPI name `1Gbps 50TB`, price `0` |
| Dedicated | `1Gbps unmetered (10000 P)` (panel) → InvAPI name `1Gbps unmetered`, price `10000` |
| Dedicated GPU (`gpu.*`) | Often unmetered / unlimited style — list with `instance_id` |
| VDS GPU (`vgpu.*`) | Often `1Gbps 50TB` style — list with `instance_id` |

Promo dedic is **`bm.v2-promo`** (not `v2-promo`). Pass `instance_id` (preset id) to list compatible plans. When two rows share a name, prefer a price hint or `traffic_plan_id`.

InvAPI: [`traffic_plans/list`](https://hostkey.com/documentation/apidocs/traffic_plans/#traffic_planslist).

```hcl
data "hostkey_traffic_plans" "nl" {
  location = "NL"
}

# Plans compatible with a dedicated preset (instance_id = preset id from hostkey_presets):
data "hostkey_presets" "promo" {
  location = "NL"
  name     = "bm.v2-promo"
}

data "hostkey_traffic_plans" "for_dedic" {
  location    = "NL"
  instance_id = data.hostkey_presets.promo.presets[0].id
  name        = "1Gbps"
}
```

## Argument Reference

### Optional

- `location` (String) DC location (recommended).
- `instance_id` (Number) Preset id for compatible plans.
- `server_id` (Number) Existing server id.
- `name` (String) Substring filter on plan name.

### Read-Only

- `traffic_plans` — list of matching plans (`id`, `name`, …).
