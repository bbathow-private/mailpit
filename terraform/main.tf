terraform {
  required_version = ">= 1.5"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }
}

provider "azurerm" {
  features {}
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

# -----------------------------------------------------------------------------
# Resource Group
# -----------------------------------------------------------------------------

resource "azurerm_resource_group" "mailpit" {
  name     = "rg-mailpit"
  location = var.location
}

# -----------------------------------------------------------------------------
# Networking
# -----------------------------------------------------------------------------

resource "azurerm_virtual_network" "mailpit" {
  name                = "vnet-mailpit"
  address_space       = ["10.0.0.0/24"]
  location            = azurerm_resource_group.mailpit.location
  resource_group_name = azurerm_resource_group.mailpit.name
}

resource "azurerm_subnet" "mailpit" {
  name                 = "snet-mailpit"
  resource_group_name  = azurerm_resource_group.mailpit.name
  virtual_network_name = azurerm_virtual_network.mailpit.name
  address_prefixes     = ["10.0.0.0/24"]
}

resource "azurerm_public_ip" "mailpit" {
  name                = "pip-mailpit"
  location            = azurerm_resource_group.mailpit.location
  resource_group_name = azurerm_resource_group.mailpit.name
  allocation_method   = "Static"
  sku                 = "Standard"
}

resource "azurerm_network_security_group" "mailpit" {
  name                = "nsg-mailpit"
  location            = azurerm_resource_group.mailpit.location
  resource_group_name = azurerm_resource_group.mailpit.name

  security_rule {
    name                       = "SSH"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefix      = var.allowed_ssh_cidr
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "SMTP"
    priority                   = 200
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "25"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "HTTPS"
    priority                   = 300
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "443"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }
}

resource "azurerm_network_interface" "mailpit" {
  name                = "nic-mailpit"
  location            = azurerm_resource_group.mailpit.location
  resource_group_name = azurerm_resource_group.mailpit.name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.mailpit.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.mailpit.id
  }
}

resource "azurerm_network_interface_security_group_association" "mailpit" {
  network_interface_id      = azurerm_network_interface.mailpit.id
  network_security_group_id = azurerm_network_security_group.mailpit.id
}

# -----------------------------------------------------------------------------
# Virtual Machine
# -----------------------------------------------------------------------------

resource "azurerm_linux_virtual_machine" "mailpit" {
  name                = "vm-mailpit"
  resource_group_name = azurerm_resource_group.mailpit.name
  location            = azurerm_resource_group.mailpit.location
  size                = var.vm_size
  admin_username      = var.admin_username

  network_interface_ids = [azurerm_network_interface.mailpit.id]

  admin_ssh_key {
    username   = var.admin_username
    public_key = var.admin_ssh_public_key
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts-gen2"
    version   = "latest"
  }

  custom_data = base64encode(templatefile("${path.module}/setup.sh.tpl", {
    domain               = var.domain
    letsencrypt_email    = var.letsencrypt_email
    cloudflare_api_token = var.cloudflare_api_token
    mailpit_version      = var.mailpit_version
    mailpit_download_url = var.mailpit_download_url
    bounce_rules         = file("${path.module}/bounce-rules.yaml")
    ui_auth_enabled      = var.ui_auth_user != "" && var.ui_auth_password != ""
    ui_auth_user         = var.ui_auth_user
    ui_auth_password     = var.ui_auth_password
  }))
}

# -----------------------------------------------------------------------------
# Cloudflare DNS
# -----------------------------------------------------------------------------

resource "cloudflare_record" "mailpit_a" {
  zone_id = var.cloudflare_zone_id
  name    = var.domain
  content = azurerm_public_ip.mailpit.ip_address
  type    = "A"
  ttl     = 300
  proxied = false # must be false for SMTP to work
}

resource "cloudflare_record" "mailpit_mx" {
  zone_id  = var.cloudflare_zone_id
  name     = var.domain
  content  = var.domain
  type     = "MX"
  priority = 10
  ttl      = 300
  proxied  = false
}
