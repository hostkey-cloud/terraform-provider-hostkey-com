terraform {
  required_providers {
    hostkey = {
      source = "registry.terraform.io/hostkey-cloud/hostkey-com"
    }
  }
}

provider "hostkey" {
  api_key = var.hostkey_api_key
}

variable "hostkey_api_key" {
  type      = string
  sensitive = true
}

variable "location_name" {
  type    = string
  default = "NL"
}

variable "preset_name" {
  type    = string
  default = "vm.pico"
}

variable "os_name" {
  type    = string
  default = "Ubuntu 22.04"
}

variable "traffic_plan_id" {
  type        = number
  default     = null
  description = "Traffic plan id, if you prefer not to use traffic_plan_name."
}

variable "traffic_plan_name" {
  type    = string
  default = "3 TB / 1 Gbps VM"
}

variable "soft_name" {
  type    = string
  default = null
}

variable "deploy_period" {
  type    = string
  default = "monthly"
}

variable "deploy_notify" {
  type    = bool
  default = true
}

variable "root_pass" {
  type      = string
  sensitive = true
}

variable "cancellation_type" {
  type    = number
  default = null
}

variable "cancellation_reason" {
  type    = string
  default = null
}

# Fresh create example (full cycle test).
resource "hostkey_server" "example" {
  preset_name       = var.preset_name
  location_name     = var.location_name
  os_name           = var.os_name
  soft_name         = var.soft_name
  traffic_plan_name = var.traffic_plan_name
  traffic_plan_id   = var.traffic_plan_id
  deploy_period     = var.deploy_period
  deploy_notify     = var.deploy_notify
  root_pass         = var.root_pass
  hostname          = "tf-pico-renamed"

  tags = {
    env     = "terraform-test"
    managed = "hostkey-provider"
  }

  cancellation_type   = 1
  cancellation_reason = "terraform full-cycle test"

  timeouts {
    create = "90m"
    delete = "30m"
  }
}

output "resolved_preset_id" {
  value = hostkey_server.example.preset_id
}

output "resolved_os_id" {
  value = hostkey_server.example.os_id
}

output "resolved_traffic_plan_id" {
  value = hostkey_server.example.traffic_plan_id
}

output "server_id" {
  value = hostkey_server.example.id
}

output "main_ipv4" {
  value = hostkey_server.example.main_ipv4
}
