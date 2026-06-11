// 速联 FastTunnel — 跨境专线代理
// 运行: node server.js
// 地址说明: 127.0.0.1 = 你的本机地址 = 所有服务跑在你自己电脑上

const http = require('http');
const https = require('https');
const fs = require('fs');
const path = require('path');
const { URL } = require('url');

// ── Configuration ─────────────────────────────────────────
const CONFIG_PATH = path.join(__dirname, 'config.json');

const defaultConfig = {
  listen_addr: '127.0.0.1:9080',
  upstream_url: '',
  remote_gateway: '',
  bind_ip: '',
  read_timeout: 30,
  write_timeout: 60,
  idle_timeout: 120,
  max_retries: 3,
  retry_delay: 500,
  upstream_to: 30,
  preserve_host: false,
  auth_token: '',
};

let config = { ...defaultConfig };
let proxyServer = null;
let stats = {
  total_requests: 0,
  active_requests: 0,
  failed_requests: 0,
  total_latency_ms: 0,
  last_request: null,
};
let logs = [];
const MAX_LOGS = 500;

// ── Config ────────────────────────────────────────────────
function loadConfig() {
  try {
    if (fs.existsSync(CONFIG_PATH)) {
      config = { ...defaultConfig, ...JSON.parse(fs.readFileSync(CONFIG_PATH, 'utf-8')) };
    }
  } catch (e) {
    console.error('Failed to load config:', e.message);
  }
}

function saveConfig(cfg) {
  config = { ...defaultConfig, ...cfg };
  fs.writeFileSync(CONFIG_PATH, JSON.stringify(config, null, 2));
}

function addLog(level, message) {
  const entry = {
    time: new Date().toLocaleTimeString('zh-CN', { hour12: false }),
    level,
    message,
  };
  logs.push(entry);
  if (logs.length > MAX_LOGS) logs.shift();
  console.log(`[${entry.time}] [${level}] ${message}`);
}

// ── HTTP Forwarder ────────────────────────────────────────
function forwardToUpstream(clientReq, clientRes) {
  const start = Date.now();
  stats.total_requests++;
  stats.active_requests++;
  stats.last_request = Date.now();

  const effectiveUpstream = config.remote_gateway || config.upstream_url;
  if (!effectiveUpstream) {
    clientRes.writeHead(502, { 'Content-Type': 'application/json' });
    clientRes.end(JSON.stringify({ error: '未配置上游地址' }));
    stats.active_requests--;
    stats.failed_requests++;
    return;
  }

  let targetUrl;
  try {
    targetUrl = new URL(effectiveUpstream + clientReq.url);
  } catch (e) {
    clientRes.writeHead(502, { 'Content-Type': 'application/json' });
    clientRes.end(JSON.stringify({ error: '上游地址格式错误' }));
    stats.active_requests--;
    stats.failed_requests++;
    return;
  }

  const options = {
    hostname: targetUrl.hostname,
    port: targetUrl.port || (targetUrl.protocol === 'https:' ? 443 : 80),
    path: targetUrl.pathname + targetUrl.search,
    method: clientReq.method,
    headers: { ...clientReq.headers },
    timeout: config.upstream_to * 1000,
    rejectUnauthorized: false, // 兼容自签名证书
  };

  // Strip headers
  (options.headers['x-forwarded-for'] || options.headers['X-Forwarded-For']) && delete options.headers['x-forwarded-for'];
  (options.headers['x-forwarded-for'] || options.headers['X-Forwarded-For']) && delete options.headers['X-Forwarded-For'];

  // Remove hop-by-hop headers
  delete options.headers['proxy-connection'];
  delete options.headers['proxy-authorization'];
  delete options.headers['connection'];
  delete options.headers['keep-alive'];
  delete options.headers['transfer-encoding'];

  // Set custom headers
  options.headers['x-proxy'] = 'api-tunnel';
  options.headers['x-client-source'] = 'dedicated-line';

  if (!config.preserve_host) {
    options.headers['host'] = targetUrl.host;
  }

  // Remove host if present 
  delete options.headers['host'];

  const transport = targetUrl.protocol === 'https:' ? https : http;

  function doRequest(attempt) {
    const proxyReq = transport.request(options, (proxyRes) => {
      clientRes.writeHead(proxyRes.statusCode, proxyRes.headers);
      proxyRes.pipe(clientRes);

      proxyRes.on('end', () => {
        stats.active_requests--;
        stats.total_latency_ms += Date.now() - start;
        addLog('INFO', `${clientReq.method} ${clientReq.url} → ${proxyRes.statusCode} (${Date.now() - start}ms)`);
      });
    });

    proxyReq.on('error', (err) => {
      if (attempt < config.max_retries) {
        const delay = config.retry_delay * Math.pow(2, attempt);
        addLog('WARN', `重试 ${attempt + 1}/${config.max_retries} (${delay}ms): ${err.message}`);
        setTimeout(() => doRequest(attempt + 1), delay);
      } else {
        stats.active_requests--;
        stats.failed_requests++;
        addLog('ERROR', `${clientReq.method} ${clientReq.url} → 失败: ${err.message}`);
        if (!clientRes.headersSent) {
          clientRes.writeHead(502, { 'Content-Type': 'application/json' });
          clientRes.end(JSON.stringify({ error: '上游不可达', detail: err.message }));
        }
      }
    });

    proxyReq.on('timeout', () => {
      proxyReq.destroy();
      if (attempt < config.max_retries) {
        const delay = config.retry_delay * Math.pow(2, attempt);
        setTimeout(() => doRequest(attempt + 1), delay);
      } else {
        stats.active_requests--;
        stats.failed_requests++;
        if (!clientRes.headersSent) {
          clientRes.writeHead(504, { 'Content-Type': 'application/json' });
          clientRes.end(JSON.stringify({ error: '上游超时' }));
        }
      }
    });

    clientReq.pipe(proxyReq);
  }

  doRequest(0);
}

// ── Management API ────────────────────────────────────────
function handleAPI(req, res) {
  res.setHeader('Content-Type', 'application/json');
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', '*');

  if (req.method === 'OPTIONS') {
    res.writeHead(204);
    res.end();
    return;
  }

  const url = req.url;

  // GET /api/stats
  if (url === '/api/stats' && req.method === 'GET') {
    const avg = stats.total_requests > 0 ? (stats.total_latency_ms / stats.total_requests) : 0;
    res.writeHead(200);
    res.end(JSON.stringify({
      total_requests: stats.total_requests,
      active_requests: stats.active_requests,
      failed_requests: stats.failed_requests,
      avg_latency_ms: avg,
      proxy_running: proxyServer !== null,
      upstream_status: 'unknown',
    }));
    return;
  }

  // GET /api/config
  if (url === '/api/config' && req.method === 'GET') {
    res.writeHead(200);
    res.end(JSON.stringify(config));
    return;
  }

  // POST /api/config
  if (url === '/api/config' && req.method === 'POST') {
    let body = '';
    req.on('data', (chunk) => (body += chunk));
    req.on('end', () => {
      try {
        const newCfg = JSON.parse(body);
        saveConfig(newCfg);
        addLog('INFO', '配置已保存');
        res.writeHead(200);
        res.end(JSON.stringify({ ok: true }));
      } catch (e) {
        res.writeHead(400);
        res.end(JSON.stringify({ error: e.message }));
      }
    });
    return;
  }

  // POST /api/proxy/start
  if (url === '/api/proxy/start' && req.method === 'POST') {
    if (proxyServer) {
      res.writeHead(400);
      res.end(JSON.stringify({ error: '代理已在运行' }));
      return;
    }
    const [host, portStr] = config.listen_addr.split(':');
    const port = parseInt(portStr) || 9080;

    proxyServer = http.createServer((creq, cres) => {
      if (creq.url === '/health') {
        cres.writeHead(200, { 'Content-Type': 'application/json' });
        cres.end(JSON.stringify({ status: 'ok', upstream: config.remote_gateway || config.upstream_url }));
        return;
      }
      if (creq.url === '/stats') {
        const avg = stats.total_requests > 0 ? (stats.total_latency_ms / stats.total_requests) : 0;
        cres.writeHead(200, { 'Content-Type': 'application/json' });
        cres.end(JSON.stringify({ total_requests: stats.total_requests, active_requests: stats.active_requests, failed_requests: stats.failed_requests, avg_latency_ms: avg }));
        return;
      }
      forwardToUpstream(creq, cres);
    });

    proxyServer.listen(port, host || '127.0.0.1', () => {
      addLog('INFO', `代理已启动: ${config.listen_addr}`);
      res.writeHead(200);
      res.end(JSON.stringify({ ok: true, addr: config.listen_addr }));
    });
    return;
  }

  // POST /api/proxy/stop
  if (url === '/api/proxy/stop' && req.method === 'POST') {
    if (!proxyServer) {
      res.writeHead(400);
      res.end(JSON.stringify({ error: '代理未运行' }));
      return;
    }
    proxyServer.close(() => {
      proxyServer = null;
      addLog('INFO', '代理已停止');
      res.writeHead(200);
      res.end(JSON.stringify({ ok: true }));
    });
    return;
  }

  // GET /api/logs
  if (url === '/api/logs' && req.method === 'GET') {
    res.writeHead(200);
    res.end(JSON.stringify(logs));
    return;
  }

  // DELETE /api/logs
  if (url === '/api/logs' && req.method === 'DELETE') {
    logs = [];
    res.writeHead(200);
    res.end(JSON.stringify({ ok: true }));
    return;
  }

  res.writeHead(404);
  res.end(JSON.stringify({ error: 'not found' }));
}

// ── Static File Server ───────────────────────────────────
const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'application/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
};

function serveStatic(req, res) {
  let filePath = req.url === '/' ? '/index.html' : req.url;
  filePath = path.join(__dirname, 'frontend', 'dist', filePath);

  const ext = path.extname(filePath);
  const contentType = MIME[ext] || 'application/octet-stream';

  fs.readFile(filePath, (err, data) => {
    if (err) {
      // SPA fallback: serve index.html for any non-file route
      fs.readFile(path.join(__dirname, 'frontend', 'dist', 'index.html'), (err2, html) => {
        if (err2) {
          res.writeHead(500);
          res.end('Internal Error');
          return;
        }
        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        res.end(html);
      });
      return;
    }
    res.writeHead(200, { 'Content-Type': contentType });
    res.end(data);
  });
}

// ── Main Server ──────────────────────────────────────────
loadConfig();
const MAIN_PORT = 8580;
const mainServer = http.createServer((req, res) => {
  if (req.url.startsWith('/api/')) {
    handleAPI(req, res);
  } else {
    serveStatic(req, res);
  }
});

mainServer.listen(MAIN_PORT, () => {
  addLog('INFO', `速联 FastTunnel 管理面板: 你的本机地址:${MAIN_PORT}`);
  addLog('INFO', `API 接口: 你的本机地址:${MAIN_PORT}/api/`);
  addLog('INFO', `就绪 — 所有地址属于使用者自己`);
});
