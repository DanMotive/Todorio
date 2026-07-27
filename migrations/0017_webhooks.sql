-- Outgoing webhooks: one row per endpoint someone has registered for a space.
--
-- The feature is inert until a row exists. No table contents means the dispatcher looks up
-- nothing and posts nowhere, which is the behaviour we want for a self-hosted install that never
-- opens this screen: an unconfigured integration must cost nothing and reach nobody.
--
-- `events` is a JSON array of event names ('task.created', 'comment.created', ...). An empty
-- array means every event, so a webhook added without ticking any box still does something
-- useful rather than silently never firing.
--
-- `secret` signs the payload (HMAC-SHA256 in the X-Todorio-Signature header). It is stored in
-- plain text on purpose: unlike a password, the server has to reproduce it to sign each delivery,
-- so it cannot be hashed. It is never returned by the API after creation.
CREATE TABLE IF NOT EXISTS webhooks (
	id               BIGSERIAL PRIMARY KEY,
	space_id         BIGINT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
	url              TEXT NOT NULL,
	secret           TEXT NOT NULL DEFAULT '',
	events           JSONB NOT NULL DEFAULT '[]'::jsonb,
	is_active        BOOLEAN NOT NULL DEFAULT TRUE,
	created_by       BIGINT REFERENCES users(id) ON DELETE SET NULL,
	created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
	-- Delivery bookkeeping. A webhook that stopped working is otherwise invisible: the owner
	-- sees a row that looks fine while the receiving end has been answering 500 for a week.
	last_status      INT,
	last_error       TEXT,
	last_delivery_at TIMESTAMPTZ,
	failure_count    INT NOT NULL DEFAULT 0
);

-- The dispatcher's only query: active hooks for one space, on every event.
CREATE INDEX IF NOT EXISTS webhooks_space_active_idx ON webhooks(space_id) WHERE is_active;
