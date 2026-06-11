#!/usr/bin/env bash
# API Tunnel — 一键部署脚本
# 支持: Ubuntu/Debian/CentOS + Docker

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# ── 检查 Docker ──
check_docker() {
    if ! command -v docker &>/dev/null; then
        warn "Docker 未安装，正在安装..."
        curl -fsSL https://get.docker.com | bash
        systemctl enable docker --now
        info "Docker 安装完成"
    else
        info "Docker 已安装: $(docker --version)"
    fi

    if ! command -v docker-compose &>/dev/null && ! docker compose version &>/dev/null; then
        warn "安装 docker-compose..."
        apt-get update -qq && apt-get install -y -qq docker-compose-plugin 2>/dev/null || true
    fi
}

# ── 创建配置 ──
create_config() {
    local upstream_url="${1:-https://api.example.com}"
    local bind_ip="${2:-}"
    local auth_token="${3:-}"

    mkdir -p config

    cat > config/config.yaml <<EOF
gateway:
  listen_addr: ":8080"
  bind_ip: "${bind_ip}"
  read_timeout: 30s
  write_timeout: 60s
  idle_timeout: 120s
  max_retries: 3
  retry_delay: 500ms
  auth_token: "${auth_token}"

upstream:
  url: "${upstream_url}"
  timeout: 30s
  preserve_host: false
  strip_headers: ["X-Forwarded-For", "X-Real-IP"]
  set_headers:
    X-Proxy: api-tunnel
    X-Client-Source: dedicated-line

log:
  level: info
  file: /var/log/api-tunnel/gateway.log
  format: json
EOF

    info "配置文件生成: config/config.yaml"
}

# ── 构建 & 启动 ──
deploy() {
    info "构建 Docker 镜像..."
    docker compose -f deploy/docker-compose.yml build --no-cache

    info "启动服务..."
    docker compose -f deploy/docker-compose.yml up -d

    info "等待服务就绪..."
    sleep 3

    if curl -s http://localhost:8080/health | grep -q ok; then
        info "✅ 部署成功! http://localhost:8080"
        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "  API Tunnel 已启动"
        echo "  健康检查: curl http://localhost:8080/health"
        echo "  查看日志: docker logs -f api-tunnel"
        echo "  停止服务: docker compose -f deploy/docker-compose.yml down"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    else
        error "❌ 服务启动失败，请检查日志: docker logs api-tunnel"
    fi
}

# ── 专线网卡模式 ──
deploy_dedicated_nic() {
    local nic="${1:-eth1}"

    warn "使用专线网卡模式: $nic"
    warn "此模式需要 network_mode: host"

    # 修改 docker-compose 使用 host 网络
    sed -i 's/# network_mode: host/network_mode: host/' deploy/docker-compose.yml

    deploy
}

# ── 主流程 ──
main() {
    echo ""
    echo "╔══════════════════════════════════════╗"
    echo "║     API Tunnel 一键部署脚本          ║"
    echo "╚══════════════════════════════════════╝"
    echo ""

    local mode="${1:-standard}"
    local upstream="${2:-https://api.example.com}"

    check_docker

    case "$mode" in
        dedicated)
            local nic="${3:-eth1}"
            create_config "$upstream" "" ""
            deploy_dedicated_nic "$nic"
            ;;
        bind-ip)
            local ip="${3:-10.0.0.100}"
            local token="${4:-}"
            create_config "$upstream" "$ip" "$token"
            deploy
            ;;
        *)
            local token="${3:-}"
            create_config "$upstream" "" "$token"
            deploy
            ;;
    esac
}

main "$@"
