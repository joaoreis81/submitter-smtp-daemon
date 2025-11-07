-- ClickHouse schema for SMTP Outbound Daemon
-- Complete event logging and analytics

CREATE DATABASE IF NOT EXISTS outbound;

USE outbound;

-- Main events table - stores all SMTP transaction events
CREATE TABLE IF NOT EXISTS outbound_events (
    -- Event identification
    event_id UUID DEFAULT generateUUIDv4(),
    event_time DateTime64(3) DEFAULT now64(3),
    msg_id String,
    session_id String,

    -- Connection details
    client_ip String,
    client_port UInt16,
    client_rdns String,
    listener_ip String,
    listener_port UInt16,

    -- TLS details
    tls_version String,
    tls_cipher String,
    sni_hostname String,
    tls_mode String,  -- starttls, implicit_tls, plaintext

    -- Authentication
    auth_user String,
    auth_method String,  -- PLAIN, LOGIN, IP
    auth_result String,  -- success, failure, rate_limited
    auth_duration_ms UInt32,

    -- Message envelope
    mail_from String,
    rcpt_to Array(String),
    message_size UInt32,

    -- Message metadata
    subject String,
    message_id String,
    date String,
    from_header String,
    to_header String,

    -- Processing pipeline
    stage String,  -- accepted, parsed, filtered, modified, signed, delivering, delivered, failed
    stage_duration_ms UInt32,

    -- Deduplication
    body_hash String,
    dedup_duplicate Bool DEFAULT false,
    dedup_original_msg_id String,

    -- DKIM signing
    dkim_enabled Bool DEFAULT false,
    dkim_signed Bool DEFAULT false,
    dkim_selector String,
    dkim_domain String,
    dkim_result String,
    dkim_error String,

    -- Modification
    modified Bool DEFAULT false,
    modification_profile String,
    headers_added Array(String),
    headers_removed Array(String),

    -- Delivery details
    recipient String,  -- Individual recipient for delivery events
    mx_hostname String,
    mx_ip String,
    mx_port UInt16,
    smtp_code UInt16,
    smtp_message String,
    delivery_duration_ms UInt32,
    delivery_result String,  -- success, temp_fail, perm_fail, deferred
    delivery_attempt UInt8,
    next_retry_time DateTime,

    -- DSN details
    dsn_ret String,
    dsn_envid String,
    dsn_notify Array(String),
    dsn_orcpt String,

    -- Performance metrics
    total_duration_ms UInt32,
    queue_wait_ms UInt32,

    -- Rate limiting
    rate_limited Bool DEFAULT false,
    rate_limit_type String,  -- ip, user, global

    -- Error tracking
    error_occurred Bool DEFAULT false,
    error_message String,
    error_code String,

    -- Additional metadata
    tags Array(String),
    custom_fields Map(String, String)

) ENGINE = MergeTree()
PARTITION BY toYYYYMM(event_time)
ORDER BY (event_time, client_ip, auth_user, msg_id)
TTL event_time + INTERVAL 90 DAY  -- Keep events for 90 days
SETTINGS index_granularity = 8192;

-- Materialized view for message summary (one row per message)
CREATE MATERIALIZED VIEW IF NOT EXISTS message_summary
ENGINE = MergeTree()
PARTITION BY toYYYYMM(event_time)
ORDER BY (event_time, auth_user, msg_id)
AS SELECT
    msg_id,
    session_id,
    min(event_time) AS accepted_time,
    max(event_time) AS completed_time,
    dateDiff('second', accepted_time, completed_time) AS total_duration_seconds,
    auth_user,
    client_ip,
    mail_from,
    rcpt_to,
    length(rcpt_to) AS recipient_count,
    message_size,
    subject,
    body_hash,
    dedup_duplicate,
    dkim_signed,
    modified,
    countIf(stage = 'delivered') AS delivered_count,
    countIf(stage = 'failed') AS failed_count,
    any(error_message) AS error_summary
FROM outbound_events
GROUP BY msg_id, session_id, auth_user, client_ip, mail_from, rcpt_to, message_size, subject, body_hash, dedup_duplicate, dkim_signed, modified;

-- Index for fast user queries
CREATE INDEX IF NOT EXISTS idx_auth_user ON outbound_events (auth_user) TYPE bloom_filter GRANULARITY 1;

-- Index for fast recipient queries
CREATE INDEX IF NOT EXISTS idx_recipient ON outbound_events (recipient) TYPE bloom_filter GRANULARITY 1;

-- Index for fast error queries
CREATE INDEX IF NOT EXISTS idx_error ON outbound_events (error_occurred) TYPE set(0) GRANULARITY 1;

-- Statistics table for quick dashboards
CREATE TABLE IF NOT EXISTS hourly_stats (
    hour DateTime,
    auth_user String,
    client_ip String,
    messages_accepted UInt32,
    messages_delivered UInt32,
    messages_failed UInt32,
    total_size UInt64,
    avg_delivery_time_ms Float32,
    unique_recipients UInt32
) ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(hour)
ORDER BY (hour, auth_user, client_ip);

-- Materialized view to populate hourly stats
CREATE MATERIALIZED VIEW IF NOT EXISTS hourly_stats_mv
TO hourly_stats
AS SELECT
    toStartOfHour(event_time) AS hour,
    auth_user,
    client_ip,
    countIf(stage = 'accepted') AS messages_accepted,
    countIf(stage = 'delivered' AND delivery_result = 'success') AS messages_delivered,
    countIf(stage = 'failed') AS messages_failed,
    sum(message_size) AS total_size,
    avg(delivery_duration_ms) AS avg_delivery_time_ms,
    uniq(recipient) AS unique_recipients
FROM outbound_events
GROUP BY hour, auth_user, client_ip;

-- Grant permissions (adjust for production)
-- GRANT SELECT, INSERT ON outbound.* TO outbound_user;
