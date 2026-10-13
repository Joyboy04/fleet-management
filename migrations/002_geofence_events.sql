-- Geofence events as consumed by the worker from the geofence_alerts queue (shown on the dashboard).
CREATE TABLE IF NOT EXISTS geofence_events (
    id          BIGSERIAL        PRIMARY KEY,
    vehicle_id  VARCHAR(32)      NOT NULL,
    event       VARCHAR(32)      NOT NULL,
    latitude    DOUBLE PRECISION NOT NULL,
    longitude   DOUBLE PRECISION NOT NULL,
    "timestamp" BIGINT           NOT NULL, -- unix seconds of the triggering reading
    received_at TIMESTAMPTZ      NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_geofence_events_id_desc ON geofence_events (id DESC);
