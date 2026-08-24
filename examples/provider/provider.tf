terraform {
  required_providers {
    hostkey = {
      source  = "hostkey-cloud/hostkey-com"
      version = "~> 0.2"
    }
  }
  required_version = ">= 1.0"
}

provider "hostkey" {
  # Prefer env: HOSTKEY_API_KEY (or HOSTKEY_API_TOKEN)
  # Endpoint is invapi.hostkey.com (use hostkey-cloud/hostkey-ru for .ru).

  # Optional knobs:
  # http_timeout = 60
  # max_retries  = 3
  # token_ttl    = 3600
  # base_url     = "https://invapi.hostkey.com/"
}
