-- Restructure ad_accounts: add account_id (act_XXXXXX) as a stable
-- field; id becomes an opaque UUID primary key.
-- The same Facebook account can appear under multiple tokens:
-- uniqueness is enforced on (account_id, token_id).
CREATE TABLE ad_accounts_new (
    id             TEXT NOT NULL PRIMARY KEY,
    account_id     TEXT NOT NULL,
    name           TEXT NOT NULL DEFAULT '',
    currency       TEXT NOT NULL DEFAULT 'USD',
    status         INTEGER NOT NULL DEFAULT 1,
    token_id       TEXT NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
    last_synced_at DATETIME,
    UNIQUE(account_id, token_id)
);

-- Existing rows: id and account_id both start as the old act_XXXXXX value.
-- New rows inserted by the application will have proper UUIDs as id.
INSERT INTO ad_accounts_new(id, account_id, name, currency, status, token_id, last_synced_at)
SELECT id, id, name, currency, status, token_id, last_synced_at
FROM ad_accounts;

-- spend_raw: account_id stores act_XXXXXX; no FK to ad_accounts since
-- account_id is no longer unique alone (same account can be under
-- multiple tokens). Data integrity is maintained at application level.
CREATE TABLE spend_raw_new (
    id            TEXT PRIMARY KEY,
    account_id    TEXT NOT NULL,
    date          TEXT NOT NULL,
    campaign_id   TEXT NOT NULL DEFAULT '',
    campaign_name TEXT NOT NULL DEFAULT '',
    adset_id      TEXT NOT NULL DEFAULT '',
    adset_name    TEXT NOT NULL DEFAULT '',
    impressions   INTEGER NOT NULL DEFAULT 0,
    clicks        INTEGER NOT NULL DEFAULT 0,
    spend         REAL NOT NULL DEFAULT 0,
    currency      TEXT NOT NULL DEFAULT 'USD',
    synced_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(account_id, date, adset_id)
);

INSERT INTO spend_raw_new(id, account_id, date, campaign_id, campaign_name,
                          adset_id, adset_name, impressions, clicks,
                          spend, currency, synced_at)
SELECT id, account_id, date, campaign_id, campaign_name,
       adset_id, adset_name, impressions, clicks,
       spend, currency, synced_at
FROM spend_raw;

-- sync_state: per account_id (no FK since account_id is not unique alone).
CREATE TABLE sync_state_new (
    account_id    TEXT PRIMARY KEY,
    last_ok_date  TEXT NOT NULL DEFAULT '',
    next_retry_at DATETIME
);

INSERT INTO sync_state_new SELECT * FROM sync_state;

DROP TABLE sync_state;
DROP TABLE spend_raw;
DROP TABLE ad_accounts;

ALTER TABLE ad_accounts_new RENAME TO ad_accounts;
ALTER TABLE spend_raw_new RENAME TO spend_raw;
ALTER TABLE sync_state_new RENAME TO sync_state;

CREATE INDEX IF NOT EXISTS idx_spend_raw_account_date ON spend_raw(account_id, date);
CREATE INDEX IF NOT EXISTS idx_spend_raw_date ON spend_raw(date);
CREATE INDEX IF NOT EXISTS idx_ad_accounts_token ON ad_accounts(token_id);
CREATE INDEX IF NOT EXISTS idx_ad_accounts_account ON ad_accounts(account_id);
