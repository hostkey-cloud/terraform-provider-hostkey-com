terraform {
  required_providers {
    hostkey = {
      source = "registry.terraform.io/hostkey-cloud/hostkey-com"
    }
  }
}

provider "hostkey" {
}

# Creates a pdns zone. Destroy calls pdns/delete_domain.
# If the zone was removed outside Terraform, the next refresh removes it from state.
resource "hostkey_dns_domain" "example" {
  name = var.domain_name
}

variable "domain_name" {
  type        = string
  description = "DNS zone name you own / are allowed to manage in Hostkey pdns"
}

# Example record (depends on domain existing in the account).
# resource "hostkey_dns_record" "www" {
#   domain = hostkey_dns_domain.example.name
#   name   = "www"
#   type   = "A"
#   content = "203.0.113.10"
#   ttl    = 300
# }
