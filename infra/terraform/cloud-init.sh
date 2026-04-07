#!/bin/bash
# Cloud-init bootstrap for vela-gateway OCI instance (Ubuntu 22.04 ARM).
# Runs once on first boot.
set -euo pipefail

# Install Docker
curl -fsSL https://get.docker.com | sh
usermod -aG docker ubuntu
systemctl enable --now docker

# Install goose for migrations
curl -fsSL https://raw.githubusercontent.com/pressly/goose/master/install.sh | sh

# Pull the vela-pulse repo (or copy the gateway Docker image via registry in production).
apt-get install -y git
git clone https://github.com/colehellman/vela-pulse /opt/vela-pulse

# Placeholder: copy .env.local from secrets manager / object storage.
# In production: use OCI Vault and a startup script to populate /opt/vela-pulse/.env.local

# Start the stack
cd /opt/vela-pulse
docker compose -f infra/compose.yaml up -d

# Run migrations (waits for Postgres to be ready).
sleep 10
cd gateway && DATABASE_URL="$(grep DATABASE_URL /opt/vela-pulse/.env.local | cut -d= -f2-)" \
  $(go env GOPATH 2>/dev/null || echo /root/go)/bin/goose \
  -dir ../infra/migrations postgres "$DATABASE_URL" up
