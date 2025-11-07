package db

const schema = `
-- Messages table
CREATE TABLE IF NOT EXISTS messages (
    msg_id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    auth_user TEXT,
    client_ip TEXT NOT NULL,
    mail_from TEXT NOT NULL,
    rcpt_to TEXT NOT NULL,  -- JSON array
    message_size INTEGER NOT NULL,
    body_hash TEXT NOT NULL,
    dedup_original TEXT,  -- Original msg_id if duplicate
    created_at INTEGER NOT NULL,
    current_stage TEXT NOT NULL,

    -- Message metadata
    subject TEXT,
    message_id_header TEXT,
    date_header TEXT,

    INDEX idx_created_at (created_at),
    INDEX idx_current_stage (current_stage),
    INDEX idx_auth_user (auth_user)
);

-- Delivery queue
CREATE TABLE IF NOT EXISTS delivery_queue (
    dlv_id TEXT PRIMARY KEY,
    msg_id TEXT NOT NULL,
    recipient TEXT NOT NULL,
    mx_hostname TEXT,
    mx_ip TEXT,
    state TEXT NOT NULL DEFAULT 'queued',  -- queued, delivering, delivered, temp_fail, perm_fail
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 7,
    next_attempt INTEGER,
    last_error TEXT,
    smtp_code INTEGER,
    smtp_message TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    delivered_at INTEGER,

    FOREIGN KEY (msg_id) REFERENCES messages(msg_id),
    INDEX idx_state (state),
    INDEX idx_next_attempt (next_attempt),
    INDEX idx_msg_id (msg_id)
);

-- DKIM keys configuration
CREATE TABLE IF NOT EXISTS dkim_keys (
    domain TEXT PRIMARY KEY,
    selector TEXT NOT NULL,
    private_key_path TEXT NOT NULL,
    active INTEGER DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- User sessions (for tracking)
CREATE TABLE IF NOT EXISTS sessions (
    session_id TEXT PRIMARY KEY,
    auth_user TEXT,
    client_ip TEXT NOT NULL,
    tls_version TEXT,
    tls_cipher TEXT,
    started_at INTEGER NOT NULL,
    ended_at INTEGER,
    messages_sent INTEGER DEFAULT 0,

    INDEX idx_auth_user (auth_user),
    INDEX idx_started_at (started_at)
);

-- Statistics cache
CREATE TABLE IF NOT EXISTS stats_cache (
    stat_key TEXT PRIMARY KEY,
    stat_value TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
`
