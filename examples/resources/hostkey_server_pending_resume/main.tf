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
  default = "RU"
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
  description = "Use an explicit traffic plan id for the chosen location/preset (e.g. 8 for RU vm.pico during local testing)."
}

variable "root_pass" {
  type      = string
  sensitive = true
}

variable "hostname" {
  type        = string
  description = "Set a unique hostname for each test run. It is used to disambiguate a new server if callback did not return server id."
}

resource "hostkey_server" "pending_resume" {
  preset_name       = var.preset_name
  location_name     = var.location_name
  os_name           = var.os_name
  traffic_plan_id   = var.traffic_plan_id
  traffic_plan_name = null
  deploy_period     = "monthly"
  deploy_notify     = true
  root_pass         = var.root_pass
  hostname          = var.hostname

  tags = {
    scenario = "pending-resume-test"
    managed  = "hostkey-provider"
  }

  cancellation_type = 1
  # Make the resume/cancellation marker unique per run to avoid re-attaching to
  # an already-existing pending order/instance in the account.
  cancellation_reason = "terraform pending resume test: ${var.hostname}"

  timeouts {
    create = "15m"
    delete = "30m"
  }
}

output "server_id" {
  value = hostkey_server.pending_resume.id
}

output "invoice" {
  value = hostkey_server.pending_resume.invoice
}

output "main_ipv4" {
  value = hostkey_server.pending_resume.main_ipv4
}

output "status" {
  value = hostkey_server.pending_resume.status
}

