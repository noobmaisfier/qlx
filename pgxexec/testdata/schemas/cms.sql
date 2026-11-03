-- schemas: cms
-- A publishing schema with the naming habits found in the wild: reserved words,
-- mixed case, plural and singular names side by side.
CREATE SCHEMA cms;
CREATE TYPE cms.post_state AS ENUM ('draft', 'review', 'published', 'archived');

CREATE TABLE cms."user" (
    id        serial PRIMARY KEY,
    handle    text NOT NULL UNIQUE,
    "Email"   text NOT NULL,
    bio       text,
    is_admin  boolean NOT NULL DEFAULT false,
    joined    date NOT NULL DEFAULT current_date
);
CREATE TABLE cms."group" (
    id   serial PRIMARY KEY,
    name text NOT NULL
);
CREATE TABLE cms.group_member (
    group_id integer NOT NULL REFERENCES cms."group" (id),
    user_id  integer NOT NULL REFERENCES cms."user" (id),
    since    date,
    PRIMARY KEY (group_id, user_id)
);
CREATE TABLE cms."order" (            -- a table called "order", as real schemas have
    id    serial PRIMARY KEY,
    "by"  text,
    "limit" integer
);
CREATE TABLE cms."BlogPost" (
    "postId"   bigserial PRIMARY KEY,
    author_id  integer NOT NULL REFERENCES cms."user" (id),
    slug       text NOT NULL,
    title      text NOT NULL,
    body       text NOT NULL DEFAULT '',
    state      cms.post_state NOT NULL DEFAULT 'draft',
    tags       text[] NOT NULL DEFAULT '{}',
    published  timestamptz,
    updated    timestamptz NOT NULL DEFAULT now(),
    meta       json,
    UNIQUE (author_id, slug)
);
CREATE TABLE cms.comment (
    id        bigserial PRIMARY KEY,
    post_id   bigint NOT NULL REFERENCES cms."BlogPost" ("postId") ON DELETE CASCADE,
    parent_id bigint REFERENCES cms.comment (id),
    user_id   integer REFERENCES cms."user" (id),
    body      text NOT NULL,
    created   timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cms.media (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id   bigint REFERENCES cms."BlogPost" ("postId"),
    mime_type text NOT NULL,
    bytes     bytea NOT NULL,
    width     integer,
    height    integer,
    size      integer GENERATED ALWAYS AS (length(bytes)) STORED
);
CREATE TABLE cms.revision (
    post_id  bigint NOT NULL REFERENCES cms."BlogPost" ("postId"),
    rev      integer NOT NULL,
    diff     jsonb NOT NULL,
    made_by  integer REFERENCES cms."user" (id),
    made_at  timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (post_id, rev)
);
CREATE TABLE cms.settings (
    key   text PRIMARY KEY,
    value jsonb NOT NULL
);
CREATE TABLE cms.page_view (          -- no primary key, like most log tables
    post_id bigint,
    at      timestamp NOT NULL DEFAULT now(),
    referer text,
    ip      inet
);
CREATE TABLE cms.post (               -- "post" next to "BlogPost" and "posts"
    id int PRIMARY KEY
);
CREATE TABLE cms.posts (
    id int PRIMARY KEY,
    post int REFERENCES cms.post (id)
);
CREATE VIEW cms.published AS
    SELECT "postId", title, published FROM cms."BlogPost" WHERE state = 'published';
