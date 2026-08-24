# Minimal paid apply — one file (vm.pico, NL).
#
#   export HOSTKEY_API_KEY="…"
#   cp terraform.tfvars.example terraform.tfvars   # set root_pass
#   terraform init && terraform apply
#
# Destroy: terraform destroy

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

output "server_id" {
  value = hostkey_server.web.id
}

output "main_ipv4" {
  value = hostkey_server.web.main_ipv4
}
