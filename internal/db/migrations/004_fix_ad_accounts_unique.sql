-- Fix ad_accounts unique constraint: replace UNIQUE(account_id) with
-- UNIQUE(account_id, token_id) so the same Facebook account can exist
-- under multiple tokens simultaneously.
--
-- Also rebuild spend_raw and sync_state to remove their FK references
-- to ad_accounts(account_id) — that column is no longer unique alone,
-- so those FKs are now invalid.

CREATE TABLE ad_accounts_fix (
    id             TEXT NOT NULL PRIMARY KEY,
    account_id     TEXT NOT NULL,
    name           TEXT NOT NULL DEFAULT '',
    currency       TEXT NOT NULL DEFAULT 'USD',
    status         INTEGER NOT NULL DEFAULT 1,
    token_id       TEXT NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
    last_synced_at DATETIME,
    UNIQUE(account_id, token_id)
);

INSERT INTO ad_accounts_fix(id, account_id, name, currency, status, token_id, last_synced_at)
SELECT id, account_id, name, currency, status, token_id, last_synced_at
FROM ad_accounts;

CREATE TABLE spend_raw_fix (
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

INSERT INTO spend_raw_fix(id, account_id, date, campaign_id, campaign_name,
                          adset_id, adset_name, impressions, clicks,
                          spend, currency, synced_at)
SELECT id, account_id, date, campaign_id, campaign_name,
       adset_id, adset_name, impressions, clicks,
       spend, currency, synced_at
FROM spend_raw;

CREATE TABLE sync_state_fix (
    account_id    TEXT PRIMARY KEY,
    last_ok_date  TEXT NOT NULL DEFAULT '',
    next_retry_at DATETIME
);

INSERT INTO sync_state_fix SELECT * FROM sync_state;

DROP TABLE sync_state;
DROP TABLE spend_raw;
DROP TABLE ad_accounts;

ALTER TABLE ad_accounts_fix  RENAME TO ad_accounts;
ALTER TABLE spend_raw_fix    RENAME TO spend_raw;
ALTER TABLE sync_state_fix   RENAME TO sync_state;

CREATE INDEX IF NOT EXISTS idx_spend_raw_account_date ON spend_raw(account_id, date);
CREATE INDEX IF NOT EXISTS idx_spend_raw_date          ON spend_raw(date);
CREATE INDEX IF NOT EXISTS idx_ad_accounts_token       ON ad_accounts(token_id);
CREATE INDEX IF NOT EXISTS idx_ad_accounts_account     ON ad_accounts(account_id);
