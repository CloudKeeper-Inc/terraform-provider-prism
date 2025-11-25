terraform {
  required_version = ">= 1.0"

  required_providers {
    prism = {
      source  = "CloudKeeper-Inc/prism"
      version = "~> 1.0"
    }
  }
}

# Configure the CloudKeeper Prism Provider
provider "prism" {
  # Subdomain for your CloudKeeper Prism instance
  prism_subdomain = var.prism_subdomain

  # Base URL for your Prism instance (without port)
  # The port 8090 is automatically appended
  base_url = var.prism_base_url

  # API token for authentication
  # Best practice: use environment variable PRISM_API_TOKEN or a variable
  api_token = var.prism_api_token
}

variable "prism_subdomain" {
  type        = string
  description = "CloudKeeper Prism subdomain"
}

variable "prism_api_token" {
  type        = string
  description = "CloudKeeper Prism API token"
  sensitive   = true
}

variable "prism_base_url" {
  type        = string
  description = "CloudKeeper Prism base URL (e.g., https://prism.cloudkeeper.com)"
}
