output "public_ip" {
  description = "VM public IP address"
  value       = azurerm_public_ip.mailpit.ip_address
}

output "web_ui_url" {
  description = "Mailpit web UI URL"
  value       = "https://${var.domain}"
}

output "smtp_host" {
  description = "SMTP endpoint for sending mail"
  value       = "${var.domain}:25"
}

output "ssh_command" {
  description = "SSH command to connect to the VM"
  value       = "ssh ${var.admin_username}@${azurerm_public_ip.mailpit.ip_address}"
}

output "bounce_rules_api" {
  description = "Bounce rules API endpoint"
  value       = "https://${var.domain}/api/v1/bounce-rules"
}
