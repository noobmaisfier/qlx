-- schemas: acct
-- Double-entry accounting: strict typing, composite keys, deferrable constraints,
-- ranges and domains.
CREATE SCHEMA acct;
CREATE DOMAIN acct.amount AS numeric(20,4);
CREATE DOMAIN acct.currency_code AS char(3) CHECK (VALUE = upper(VALUE));
CREATE TYPE acct.account_kind AS ENUM ('asset', 'liability', 'equity', 'income', 'expense');

CREATE TABLE acct.currencies (
    code     acct.currency_code PRIMARY KEY,
    name     text NOT NULL,
    decimals smallint NOT NULL DEFAULT 2
);
CREATE TABLE acct.accounts (
    code      text PRIMARY KEY,
    parent    text REFERENCES acct.accounts (code) DEFERRABLE INITIALLY DEFERRED,
    name      text NOT NULL,
    kind      acct.account_kind NOT NULL,
    currency  acct.currency_code NOT NULL REFERENCES acct.currencies (code),
    closed_on date
);
CREATE TABLE acct.fiscal_periods (
    id      integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    label   text NOT NULL UNIQUE,
    span    daterange NOT NULL,
    locked  boolean NOT NULL DEFAULT false
);
CREATE TABLE acct.entries (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    period_id  integer NOT NULL REFERENCES acct.fiscal_periods (id),
    posted_on  date NOT NULL,
    memo       text,
    external   uuid,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE acct.lines (
    entry_id bigint NOT NULL REFERENCES acct.entries (id) ON DELETE CASCADE,
    line_no  integer NOT NULL,
    account  text NOT NULL REFERENCES acct.accounts (code),
    debit    acct.amount NOT NULL DEFAULT 0,
    credit   acct.amount NOT NULL DEFAULT 0,
    net      acct.amount GENERATED ALWAYS AS (debit - credit) STORED,
    PRIMARY KEY (entry_id, line_no),
    CHECK (debit >= 0 AND credit >= 0 AND (debit = 0 OR credit = 0))
);
CREATE TABLE acct.exchange_rates (
    base     acct.currency_code NOT NULL REFERENCES acct.currencies (code),
    quote    acct.currency_code NOT NULL REFERENCES acct.currencies (code),
    valid    tstzrange NOT NULL,
    rate     numeric(18,8) NOT NULL CHECK (rate > 0),
    PRIMARY KEY (base, quote, valid)
);
CREATE TABLE acct.attachments (
    entry_id bigint NOT NULL,
    line_no  integer NOT NULL,
    note     text,
    FOREIGN KEY (entry_id, line_no) REFERENCES acct.lines (entry_id, line_no)
);
CREATE MATERIALIZED VIEW acct.balances AS
    SELECT account, sum(net) AS balance FROM acct.lines GROUP BY account;
CREATE UNIQUE INDEX balances_account ON acct.balances (account);
