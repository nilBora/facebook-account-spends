CREATE TABLE IF NOT EXISTS tokens (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    access_token TEXT NOT NULL,
    is_active    INTEGER NOT NULL DEFAULT 1,
    expires_at   TIMESTAMPTZ,
    last_error   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ad_accounts (
    id             TEXT NOT NULL PRIMARY KEY,
    account_id     TEXT NOT NULL,
    name           TEXT NOT NULL DEFAULT '',
    currency       TEXT NOT NULL DEFAULT 'USD',
    status         INTEGER NOT NULL DEFAULT 1,
    token_id       TEXT NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
    last_synced_at TIMESTAMPTZ,
    UNIQUE(account_id, token_id)
);

CREATE TABLE IF NOT EXISTS spend_raw (
    id            TEXT PRIMARY KEY,
    account_id    TEXT NOT NULL,
    date          TEXT NOT NULL,
    campaign_id   TEXT NOT NULL DEFAULT '',
    campaign_name TEXT NOT NULL DEFAULT '',
    adset_id      TEXT NOT NULL DEFAULT '',
    adset_name    TEXT NOT NULL DEFAULT '',
    impressions   BIGINT NOT NULL DEFAULT 0,
    clicks        BIGINT NOT NULL DEFAULT 0,
    spend         REAL NOT NULL DEFAULT 0,
    currency      TEXT NOT NULL DEFAULT 'USD',
    synced_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(account_id, date, adset_id)
);

CREATE TABLE IF NOT EXISTS sync_state (
    account_id    TEXT PRIMARY KEY,
    last_ok_date  TEXT NOT NULL DEFAULT '',
    next_retry_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS fx_rates (
    date         TEXT NOT NULL,
    currency     TEXT NOT NULL,
    rate_to_usd  REAL NOT NULL,
    PRIMARY KEY (date, currency)
);

CREATE INDEX IF NOT EXISTS idx_spend_raw_account_date ON spend_raw(account_id, date);
CREATE INDEX IF NOT EXISTS idx_spend_raw_date          ON spend_raw(date);
CREATE INDEX IF NOT EXISTS idx_ad_accounts_token       ON ad_accounts(token_id);
CREATE INDEX IF NOT EXISTS idx_ad_accounts_account     ON ad_accounts(account_id);
