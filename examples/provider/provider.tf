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
  # Example: if your URL is https://acme.prism.cloudkeeper.com, use "acme"
  prism_subdomain = "your-subdomain"

  # API token for authentication
  # Best practice: use environment variable PRISM_API_TOKEN or a variable
  api_token = var.prism_api_token
}

variable "prism_api_token" {
  type        = string
  description = "CloudKeeper Prism API token"
  sensitive   = true
}
