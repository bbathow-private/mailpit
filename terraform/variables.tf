# -----------------------------------------------------------------------------
# Azure
# -----------------------------------------------------------------------------

variable "location" {
  description = "Azure region"
  type        = string
  default     = "germanywestcentral"
}

variable "vm_size" {
  description = "Azure VM size"
  type        = string
  default     = "Standard_B1s"
}

variable "admin_username" {
  description = "SSH admin username"
  type        = string
  default     = "azureuser"
}

variable "admin_ssh_public_key" {
  description = "SSH public key for VM access"
  type        = string
}

variable "allowed_ssh_cidr" {
  description = "CIDR allowed to SSH into the VM (e.g. your IP)"
  type        = string
  default     = "*"
}

# -----------------------------------------------------------------------------
# Cloudflare
# -----------------------------------------------------------------------------

variable "cloudflare_api_token" {
  description = "Cloudflare API token (needs DNS edit + zone read)"
  type        = string
  sensitive   = true
}

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID for the parent domain"
  type        = string
}

# -----------------------------------------------------------------------------
# Domain & TLS
# -----------------------------------------------------------------------------

variable "domain" {
  description = "FQDN for mailpit (e.g. mailpit.example.com)"
  type        = string
}

variable "letsencrypt_email" {
  description = "Email address for Let's Encrypt certificate notifications"
  type        = string
}

# -----------------------------------------------------------------------------
# Mailpit
# -----------------------------------------------------------------------------

variable "mailpit_version" {
  description = "Mailpit release version to install (e.g. 1.21.8). Set to empty string and provide mailpit_download_url for custom builds."
  type        = string
  default     = "0.1.0"
}

variable "mailpit_download_url" {
  description = "Override download URL for a custom mailpit binary (tar.gz with 'mailpit' inside). Takes precedence over mailpit_version."
  type        = string
  default     = ""
}

variable "ui_auth_user" {
  description = "Username for web UI basic auth (leave empty to disable auth)"
  type        = string
  default     = ""
}

variable "ui_auth_password" {
  description = "Password for web UI basic auth (leave empty to disable auth)"
  type        = string
  default     = ""
  sensitive   = true
}
