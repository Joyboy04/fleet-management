-- Table name follows the technical test spec exactly ("vehicle_loctions").
CREATE TABLE IF NOT EXISTS vehicle_loctions (
    id          BIGSERIAL        PRIMARY KEY,
    vehicle_id  VARCHAR(32)      NOT NULL,
    latitude    DOUBLE PRECISION NOT NULL,
    longitude   DOUBLE PRECISION NOT NULL,
    "timestamp" BIGINT           NOT NULL, -- unix seconds, as sent by the vehicle
    created_at  TIMESTAMPTZ      NOT NULL DEFAULT now()
);

-- Serves both "last location" and "history in range" queries.
CREATE INDEX IF NOT EXISTS idx_vehicle_loctions_vehicle_ts
    ON vehicle_loctions (vehicle_id, "timestamp" DESC);
