-- Venom Tunnel SQLite Schema (WAL Mode)
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS panel_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tunnels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    engine TEXT NOT NULL,            -- backhaul-wssmux, backhaul-tcpmux, backhaul-tcp, xray-reality, rathole, gost-mwss, frp-ws, gre, wg
    role TEXT NOT NULL,              -- iran | foreign
    local_ip TEXT NOT NULL,
    remote_ip TEXT NOT NULL,
    control_port INTEGER NOT NULL DEFAULT 443,
    ports TEXT NOT NULL DEFAULT '',
    token TEXT NOT NULL DEFAULT '',
    extra_json TEXT NOT NULL DEFAULT '{}',
    service_name TEXT NOT NULL,
    config_path TEXT NOT NULL,
    imported INTEGER NOT NULL DEFAULT 0,
    auto_watchdog INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS metrics_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cpu_pct REAL NOT NULL,
    ram_pct REAL NOT NULL,
    rx_mbps REAL NOT NULL,
    tx_mbps REAL NOT NULL,
    rtt_ms REAL NOT NULL DEFAULT 0,
    intl_ok INTEGER NOT NULL DEFAULT 1,
    recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_metrics_recorded_at ON metrics_history(recorded_at);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor TEXT NOT NULL DEFAULT 'admin',
    action TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    ip_addr TEXT NOT NULL DEFAULT '',
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
