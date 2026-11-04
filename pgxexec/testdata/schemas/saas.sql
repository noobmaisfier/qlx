-- schemas: app
-- qlx: timestamptz: qt
-- qlx: array_nulls: true
-- Multi-tenant SaaS: composite foreign keys that carry the tenant, soft deletes,
-- arrays, intervals, a partitioned event table and an outbox with no key.
CREATE SCHEMA app;
CREATE TYPE app.role AS ENUM ('owner', 'admin', 'member', 'viewer');
CREATE TYPE app.task_status AS ENUM ('todo', 'doing', 'blocked', 'done');

CREATE TABLE app.tenants (
    id         integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug       text NOT NULL UNIQUE,
    plan       text NOT NULL DEFAULT 'free',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE app.users (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email      text NOT NULL UNIQUE,
    name       text,
    password   bytea,
    locale     text NOT NULL DEFAULT 'en',
    last_seen  timestamptz
);
CREATE TABLE app.memberships (
    tenant_id integer NOT NULL REFERENCES app.tenants (id),
    user_id   bigint  NOT NULL REFERENCES app.users (id),
    role      app.role NOT NULL DEFAULT 'member',
    PRIMARY KEY (tenant_id, user_id)
);
CREATE TABLE app.projects (
    tenant_id integer NOT NULL REFERENCES app.tenants (id),
    id        integer NOT NULL,
    name      text NOT NULL,
    archived  boolean NOT NULL DEFAULT false,
    PRIMARY KEY (tenant_id, id)
);
CREATE TABLE app.tasks (
    tenant_id  integer NOT NULL,
    id         bigint GENERATED ALWAYS AS IDENTITY,
    project_id integer NOT NULL,
    title      text NOT NULL,
    status     app.task_status NOT NULL DEFAULT 'todo',
    priority   smallint NOT NULL DEFAULT 3,
    due        date,
    estimate   interval,
    assignee   bigint REFERENCES app.users (id),
    labels     text[] NOT NULL DEFAULT '{}',
    metadata   jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, project_id) REFERENCES app.projects (tenant_id, id)
);
CREATE INDEX tasks_open ON app.tasks (tenant_id, due) WHERE deleted_at IS NULL AND status <> 'done';
CREATE TABLE app.api_keys (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   integer NOT NULL REFERENCES app.tenants (id),
    key_hash    bytea NOT NULL UNIQUE,
    allowed_ips inet[] NOT NULL DEFAULT '{}',
    scopes      text[] NOT NULL DEFAULT ARRAY['read'],
    expires_at  timestamptz
);
CREATE TABLE app.feature_flags (
    tenant_id integer REFERENCES app.tenants (id),
    name      text NOT NULL,
    enabled   boolean NOT NULL DEFAULT false,
    config    jsonb,
    UNIQUE NULLS NOT DISTINCT (tenant_id, name)
);
CREATE TABLE app.events (
    tenant_id  integer NOT NULL,
    at         timestamptz NOT NULL,
    kind       text NOT NULL,
    payload    jsonb
) PARTITION BY RANGE (at);
CREATE TABLE app.events_default PARTITION OF app.events DEFAULT;
CREATE TABLE app.outbox (
    topic   text NOT NULL,
    body    jsonb NOT NULL,
    created timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE app.schema_migrations (version bigint PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());

-- Rows that real data has and Go types often cannot hold: infinite timestamps
-- and NULL elements inside arrays. With `timestamptz: qt` and `array_nulls: true`
-- the program must read them back.
INSERT INTO app.tenants (slug) VALUES ('acme');
INSERT INTO app.api_keys (tenant_id, key_hash, expires_at, allowed_ips, scopes)
    VALUES (1, '\x01', 'infinity', ARRAY['10.0.0.1'::inet, NULL], ARRAY['read', NULL]),
           (1, '\x02', '-infinity', '{}', '{}'),
           (1, '\x03', NULL, '{}', '{}');
INSERT INTO app.projects (tenant_id, id, name) VALUES (1, 1, 'p');
INSERT INTO app.tasks (tenant_id, project_id, title, labels) VALUES (1, 1, 't', ARRAY['a', NULL, 'c']);
