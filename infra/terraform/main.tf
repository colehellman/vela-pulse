terraform {
  required_providers {
    oci = {
      source  = "oracle/oci"
      version = "~> 6.0"
    }
  }
  # Store state remotely once you have an OCI bucket:
  # backend "s3" {
  #   bucket   = "vela-tfstate"
  #   key      = "vela-pulse/terraform.tfstate"
  #   region   = "us-ashburn-1"
  #   endpoint = "https://<namespace>.compat.objectstorage.us-ashburn-1.oraclecloud.com"
  # }
}

provider "oci" {
  tenancy_ocid     = var.tenancy_ocid
  user_ocid        = var.user_ocid
  fingerprint      = var.fingerprint
  private_key_path = var.private_key_path
  region           = var.region
}

# ---------------------------------------------------------------------------
# Networking
# ---------------------------------------------------------------------------

resource "oci_core_vcn" "vela" {
  compartment_id = var.compartment_ocid
  display_name   = "vela-vcn"
  cidr_blocks    = ["10.0.0.0/16"]
  dns_label      = "vela"
}

resource "oci_core_internet_gateway" "igw" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.vela.id
  display_name   = "vela-igw"
  enabled        = true
}

resource "oci_core_route_table" "public" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.vela.id
  display_name   = "vela-public-rt"

  route_rules {
    destination       = "0.0.0.0/0"
    network_entity_id = oci_core_internet_gateway.igw.id
  }
}

resource "oci_core_security_list" "gateway" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.vela.id
  display_name   = "vela-gateway-sl"

  # Allow inbound HTTPS (Cloudflare terminates TLS; only CF IPs in production).
  ingress_security_rules {
    protocol  = "6" # TCP
    source    = "0.0.0.0/0"
    stateless = false
    tcp_options { max = 443; min = 443 }
  }

  # Allow inbound HTTP for ACME challenges.
  ingress_security_rules {
    protocol  = "6"
    source    = "0.0.0.0/0"
    stateless = false
    tcp_options { max = 80; min = 80 }
  }

  # SSH — restrict to your IP in production.
  ingress_security_rules {
    protocol  = "6"
    source    = "0.0.0.0/0"
    stateless = false
    tcp_options { max = 22; min = 22 }
  }

  egress_security_rules {
    protocol    = "all"
    destination = "0.0.0.0/0"
    stateless   = false
  }
}

resource "oci_core_subnet" "public" {
  compartment_id             = var.compartment_ocid
  vcn_id                     = oci_core_vcn.vela.id
  display_name               = "vela-public-subnet"
  cidr_block                 = "10.0.1.0/24"
  route_table_id             = oci_core_route_table.public.id
  security_list_ids          = [oci_core_security_list.gateway.id]
  prohibit_public_ip_on_vnic = false
  dns_label                  = "pub"
}

# ---------------------------------------------------------------------------
# Compute (ARM free-tier: up to 4 OCPUs / 24 GB across all A1 instances)
# ---------------------------------------------------------------------------

data "oci_core_images" "ubuntu_arm" {
  compartment_id           = var.compartment_ocid
  operating_system         = "Canonical Ubuntu"
  operating_system_version = "22.04"
  shape                    = var.instance_shape
  sort_by                  = "TIMECREATED"
  sort_order               = "DESC"
}

resource "oci_core_instance" "gateway" {
  compartment_id      = var.compartment_ocid
  availability_domain = data.oci_identity_availability_domains.ads.availability_domains[0].name
  display_name        = "vela-gateway"
  shape               = var.instance_shape

  shape_config {
    ocpus         = var.instance_ocpus
    memory_in_gbs = var.instance_memory_gb
  }

  source_details {
    source_type             = "image"
    source_id               = data.oci_core_images.ubuntu_arm.images[0].id
    boot_volume_size_in_gbs = 50
  }

  create_vnic_details {
    subnet_id        = oci_core_subnet.public.id
    assign_public_ip = true
    display_name     = "vela-gateway-vnic"
  }

  metadata = {
    ssh_authorized_keys = var.ssh_public_key
    # Cloud-init: install Docker, docker-compose, copy env, start services.
    user_data = base64encode(file("${path.module}/cloud-init.sh"))
  }
}

data "oci_identity_availability_domains" "ads" {
  compartment_id = var.tenancy_ocid
}

# ---------------------------------------------------------------------------
# Outputs
# ---------------------------------------------------------------------------

output "gateway_public_ip" {
  value       = oci_core_instance.gateway.public_ip
  description = "Point your Cloudflare DNS A record here."
}

output "gateway_ssh" {
  value = "ssh ubuntu@${oci_core_instance.gateway.public_ip}"
}
