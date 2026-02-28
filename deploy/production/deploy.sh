#!/usr/bin/env bash
# ============================================================================
# HiveMind Production Deploy Script
# ============================================================================
# Usage: ./deploy.sh [up|down|restart|logs|status]
#
# Prerequisites:
#   - Docker & Docker Compose v2
#   - NVIDIA Container Toolkit (for GPU support)
#   - TLS certs in ./certs/ (fullchain.pem, privkey.pem)
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.yml"
PROJECT_NAME="hivemind"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[hivemind]${NC} $*"; }
warn() { echo -e "${YELLOW}[hivemind]${NC} $*"; }
error() { echo -e "${RED}[hivemind]${NC} $*" >&2; }

check_prerequisites() {
    command -v docker >/dev/null 2>&1 || { error "docker not found"; exit 1; }
    docker compose version >/dev/null 2>&1 || { error "docker compose v2 not found"; exit 1; }

    if [ ! -f "${SCRIPT_DIR}/certs/fullchain.pem" ]; then
        warn "TLS certs not found in ${SCRIPT_DIR}/certs/"
        warn "Create self-signed certs for testing:"
        warn "  mkdir -p certs && openssl req -x509 -nodes -days 365 \\"
        warn "    -newkey rsa:2048 -keyout certs/privkey.pem -out certs/fullchain.pem \\"
        warn "    -subj '/CN=localhost'"
        echo ""
    fi
}

cmd_up() {
    check_prerequisites
    log "Starting HiveMind production stack..."
    docker compose -f "${COMPOSE_FILE}" -p "${PROJECT_NAME}" up -d --build
    log "Stack started. Waiting for health checks..."
    sleep 10
    cmd_status
}

cmd_down() {
    log "Stopping HiveMind production stack..."
    docker compose -f "${COMPOSE_FILE}" -p "${PROJECT_NAME}" down
    log "Stack stopped."
}

cmd_restart() {
    log "Restarting HiveMind production stack..."
    docker compose -f "${COMPOSE_FILE}" -p "${PROJECT_NAME}" restart
    sleep 5
    cmd_status
}

cmd_logs() {
    docker compose -f "${COMPOSE_FILE}" -p "${PROJECT_NAME}" logs -f --tail=100 "${@}"
}

cmd_status() {
    echo ""
    log "=== HiveMind Production Status ==="
    echo ""
    docker compose -f "${COMPOSE_FILE}" -p "${PROJECT_NAME}" ps
    echo ""

    # Health check
    for instance in hivemind-1 hivemind-2; do
        local health
        health=$(docker inspect --format='{{.State.Health.Status}}' "${PROJECT_NAME}-${instance}-1" 2>/dev/null || echo "unknown")
        if [ "${health}" = "healthy" ]; then
            log "${instance}: ${GREEN}healthy${NC}"
        else
            warn "${instance}: ${YELLOW}${health}${NC}"
        fi
    done
    echo ""
}

cmd_scale() {
    local count="${1:-2}"
    log "Scaling HiveMind to ${count} instances..."
    docker compose -f "${COMPOSE_FILE}" -p "${PROJECT_NAME}" up -d --scale hivemind-1="${count}"
    log "Scaled to ${count} instances."
}

# ── Main ────────────────────────────────────────────────────────────────────
case "${1:-help}" in
    up)      cmd_up ;;
    down)    cmd_down ;;
    restart) cmd_restart ;;
    logs)    shift; cmd_logs "$@" ;;
    status)  cmd_status ;;
    scale)   cmd_scale "${2:-2}" ;;
    *)
        echo "Usage: $0 {up|down|restart|logs|status|scale N}"
        echo ""
        echo "Commands:"
        echo "  up       Build and start the production stack"
        echo "  down     Stop and remove all containers"
        echo "  restart  Restart all services"
        echo "  logs     Follow container logs (optional: service name)"
        echo "  status   Show service health and status"
        echo "  scale N  Scale HiveMind instances to N"
        exit 1
        ;;
esac
