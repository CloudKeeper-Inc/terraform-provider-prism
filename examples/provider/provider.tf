terraform {
  required_providers {
    prism = {
      source = "CloudKeeper-Inc/prism"
    }
  }
}

provider "prism" {
  prism_subdomain  = "YOUR_SUBDOMAIN"
  api_token = var.prism_token
}

variable "prism_token" {
  type        = string
  description = "CloudKeeper API token"
  sensitive   = true
}
