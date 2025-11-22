terraform {
  required_providers {
    cloudkeeper = {
      source = "cloudkeeper/cloudkeeper"
    }
  }
}

provider "cloudkeeper" {
  prism_subdomain  = "YOUR_SUBDOMAIN"
  api_token = var.prism_token
}

variable "prism_token" {
  type        = string
  description = "CloudKeeper API token"
  sensitive   = true
}
