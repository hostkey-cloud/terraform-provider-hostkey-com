terraform {
  required_providers {
    hostkey = {
      source = "registry.terraform.io/hostkey-cloud/hostkey-com"
    }
  }
}

provider "hostkey" {
}

data "hostkey_presets" "nl" {
  location = "NL"
}

data "hostkey_oses" "nl" {
  location = "NL"
}

data "hostkey_traffic_plans" "nl" {
  location = "NL"
}

output "preset_names" {
  value = [for p in data.hostkey_presets.nl.presets : p.name]
}

output "os_names" {
  value = [for o in data.hostkey_oses.nl.oses : o.name]
}
