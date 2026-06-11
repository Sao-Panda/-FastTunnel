# 速联 FastTunnel

> 跨境专线 API 代理 — 一键部署，双击即用

## 是什么

速联 (FastTunnel) 是一款跨境 API 代理工具，让你的业务请求通过固定 IP 访问海外 API（如 Stripe、Google、PayPal 等），解决 IP 限制和跨境访问问题。

## 架构

```
你的应用 ──→ 本地代理(127.0.0.1:9080) ──→ [可选]远程网关(固定IP) ──→ 海外API
```

## 快速开始

### Windows 桌面版（最简单）

```bash
# 1. 解压 FastTunnel.zip
# 2. 双击 启动速联.bat
# 3. 浏览器自动打开 → 设置上游API → 启动代理
```

### 远程网关（Docker 部署到 VPS）

```bash
git clone https://github.com/Sao-Panda/-FastTunnel.git
cd -FastTunnel/gateway
bash deploy.sh standard "https://你的上游API地址"
```

## 功能

- **本地代理** — 一键启停，实时流量统计
- **远程网关** — 部署在美国 VPS，固定 IP 出站
- **两跳模式** — 本地 → 远程网关 → 上游 API
- **网卡绑定** — 支持绑定指定网卡/IP
- **超时重试** — 可配置重试次数 + 指数退避
- **认证令牌** — Bearer Token 防盗用
- **实时日志** — 级别过滤、自动滚动

## 目录

```
-FastTunnel/
├── 启动速联.bat          # Windows 一键启动
├── server.js              # 代理引擎 (Node.js)
├── frontend/dist/         # 管理面板网页
├── gateway/               # 远程网关 (Go)
│   ├── cmd/gateway/
│   ├── config/
│   ├── pkg/
│   └── deploy/
└── FastTunnel.zip         # 免安装打包
```

## 要求

- **桌面版**：Node.js 18+
- **远程网关**：Docker 或 Go 1.22+

## License

MIT
