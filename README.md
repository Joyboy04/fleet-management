# Fleet Management Backend — Transjakarta

A Go backend for a fleet management system. It:

1. Receives vehicle locations over **MQTT** (Eclipse Mosquitto) on `/fleet/vehicle/{vehicle_id}/location`.
2. Validates and stores them in **PostgreSQL** (table `vehicle_loctions`).
3. Serves a **REST API** (Gin) for a vehicle's last location and its history in a time range.
4. Publishes a `geofence_entry` event to **RabbitMQ** (exchange `fleet.events`, queue `geofence_alerts`) when a vehicle comes within **50 m** of a configured point. A **worker** service consumes that queue.
5. Runs everything with **Docker Compose**, including a mock publisher that sends a location every **2 seconds**.
6. Includes a simple **web dashboard** at http://localhost:8080/ that shows every feature on one page.

## Architecture

```
 ┌──────────────┐  MQTT publish every 2s       ┌──────────────┐
 │  publisher   │ ───────────────────────────▶ │  mosquitto   │
 │ (mock GPS)   │  /fleet/vehicle/{id}/location│ (MQTT broker)│
 └──────────────┘                              └──────┬───────┘
                                                      │ subscribe /fleet/vehicle/+/location
                                                      ▼
 ┌─────────────────────────────── api ───────────────────────────────┐
 │  MQTT subscriber ─▶ validate JSON ─▶ INSERT ─▶ geofence check     │
 │                                        │        (haversine ≤ 50 m) │
 │  Gin REST API ◀── SELECT ──────────────┤                │          │
 └────────▲───────────────────────────────┼────────────────┼──────────┘
          │ HTTP :8080                    ▼                │ publish (on entry)
     Postman / curl               ┌──────────────┐         ▼
                                  │  postgres    │  ┌─────────────────────────────┐
                                  │ vehicle_     │  │ rabbitmq                    │
                                  │ loctions     │  │ exchange fleet.events       │
                                  └──────────────┘  │  └─▶ queue geofence_alerts  │
                                                    └──────────────┬──────────────┘
                                                                   │ consume
                                                            ┌──────▼──────┐
                                                            │   worker    │
                                                            └─────────────┘
```

| Service     | Description                                                       | Port(s)                         |
|-------------|-------------------------------------------------------------------|---------------------------------|
| `api`       | MQTT subscriber + PostgreSQL writer + geofence detector + REST API | `8080`                          |
| `worker`    | Consumes events from `geofence_alerts`; logs them and saves them in `geofence_events` | —        |
| `publisher` | Mock script that publishes vehicle locations every 2 s            | —                               |
| `postgres`  | PostgreSQL 16 (user/pass/db: `fleet`)                             | `5432`                          |
| `mosquitto` | Eclipse Mosquitto 2 MQTT broker                                   | `1883`                          |
| `rabbitmq`  | RabbitMQ 3 with management UI (user/pass: `fleet`)                | `5672`, UI `15672`              |

### Flow details

- **MQTT ingestion**: the API subscribes to `/fleet/vehicle/+/location` (QoS 1). Each message is validated before it is stored:
  - the JSON is well formed and contains all four fields: `vehicle_id`, `latitude`, `longitude`, `timestamp`;
  - `latitude` is in [-90, 90], `longitude` is in [-180, 180], and `timestamp` is a positive Unix time in seconds;
  - `vehicle_id` matches `^[A-Za-z0-9_-]{1,32}$` and is the same as the `{vehicle_id}` in the topic.

  Invalid messages are logged and dropped.
- **Storage**: valid readings are inserted into `vehicle_loctions (vehicle_id, latitude, longitude, timestamp)`. The table name uses the spec's spelling. The schema lives in [migrations/001_init.sql](migrations/001_init.sql) and the API applies it on startup.
- **Geofence**: the API computes the haversine distance to the configured point. It publishes an event only on *entry*, meaning the vehicle was outside (or unknown) and is now inside the 50 m radius. A vehicle that stays inside does not trigger repeated events.
- **RabbitMQ**: `fleet.events` is a durable `direct` exchange. `geofence_alerts` is a durable queue bound with routing key `geofence.entry`. Messages are persistent and use publisher confirms. The message format matches the spec:

  ```json
  {
    "vehicle_id": "B1234XYZ",
    "event": "geofence_entry",
    "location": { "latitude": -6.2088, "longitude": 106.8456 },
    "timestamp": 1715003456
  }
  ```

## Quick start

Requirements: Docker with the Compose plugin.

```bash
docker compose up --build
```

This starts all six services. The `publisher` starts only after the `api` is healthy, so no early readings are lost. Data then starts flowing within a few seconds:

- `api` logs `geofence entry published` and `worker` logs `geofence alert`. With the default settings, each mock vehicle drives a ~1.3 km loop that passes the geofence center (Monas, `-6.1754, 106.8272`), so every vehicle triggers about one entry event per minute.
- Stop everything with `docker compose down`. Add `-v` to also delete the database volume.

## Web dashboard

Open **http://localhost:8080/** after `docker compose up`. The single page uses only the REST API, and each panel shows the endpoint it calls:

| Panel | What it shows | Endpoint |
|---|---|---|
| **Vehicles** | Every vehicle seen over MQTT, refreshed every 2 s and drawn on the map | `GET /vehicles`, `GET /vehicles/{id}/location` |
| **Latest location** | The selected vehicle's last reading (raw JSON included) | `GET /vehicles/{id}/location` |
| **Location history** | Readings in a chosen time range, as a table and as a route on the map | `GET /vehicles/{id}/history?start=&end=` |
| **Geofence events** | The 50 m zone (circle on the map) and the events the worker consumed from `geofence_alerts`; new events flash and show a toast | `GET /geofence`, `GET /geofence/events` |

The page gets its own access token using the dev client credentials from `docker-compose.yml`. The map uses OpenStreetMap tiles, so it needs internet access.

## REST API

Base URL: `http://localhost:8080`

The vehicle endpoints are protected with an **OAuth2 client-credentials** Bearer token (HS256 JWT). Get a token first:

```bash
curl -s -X POST http://localhost:8080/oauth/token \
  -d grant_type=client_credentials -d client_id=fleet-client -d client_secret=fleet-secret
# {"access_token":"eyJ...","expires_in":3600,"token_type":"Bearer"}
TOKEN=<access_token>
```

### `GET /vehicles/{vehicle_id}/location`: latest location

```bash
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/vehicles/B1234XYZ/location
```

```json
{ "vehicle_id": "B1234XYZ", "latitude": -6.2088, "longitude": 106.8456, "timestamp": 1715003456 }
```

### `GET /vehicles/{vehicle_id}/history?start=...&end=...`: history in a time range

`start` and `end` are Unix seconds, and both bounds are inclusive. The response is a JSON array ordered oldest first, or `[]` when nothing matches.

```bash
END=$(date +%s); START=$((END-600))
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/vehicles/B1234XYZ/history?start=$START&end=$END"
```

```json
[
  { "vehicle_id": "B1234XYZ", "latitude": -6.1754, "longitude": 106.827562, "timestamp": 1715003456 },
  { "vehicle_id": "B1234XYZ", "latitude": -6.1754, "longitude": 106.827924, "timestamp": 1715003458 }
]
```

> The mock publisher uses the **current** time, so the spec's example range (`start=1715000000&end=1715009999`) returns `[]` unless you publish data with those timestamps yourself (see [Manual MQTT test](#manual-mqtt-test)).

### Errors

| Status | When                                                                          |
|--------|-------------------------------------------------------------------------------|
| `400`  | `start` or `end` missing or not an integer, `start > end`, or invalid `vehicle_id` |
| `401`  | Missing, invalid or expired Bearer token                                      |
| `404`  | No location stored for this vehicle                                           |

Error body: `{"error":"not_found","message":"no location found for this vehicle"}`

Other endpoints:

| Endpoint | Auth | Description |
|---|---|---|
| `GET /` | public | Web dashboard |
| `GET /healthz` | public | Liveness |
| `POST /oauth/token` | public | Issue an access token (client credentials) |
| `GET /vehicles` | Bearer | Ids of all vehicles that have reported a location, e.g. `["B1234XYZ","B5678ABC"]` |
| `GET /geofence` | Bearer | Configured zone: `{"name":"Monas","latitude":-6.1754,"longitude":106.8272,"radius_m":50}` |
| `GET /geofence/events?limit=20` | Bearer | Latest geofence events consumed by the worker, newest first (`limit` 1–100) |

### Postman

Import [postman/fleet-management.postman_collection.json](postman/fleet-management.postman_collection.json). The collection gets a token automatically, so you can send any request directly. The "history (last 1 hour)" request computes `start` and `end` for you.

## Verifying each requirement

**1. MQTT publishing.** Watch the mock data on the broker:

```bash
docker compose logs -f publisher
docker compose exec mosquitto mosquitto_sub -t '/fleet/vehicle/+/location' -v
```

**2. PostgreSQL storage.**

```bash
docker compose exec postgres psql -U fleet -d fleet -c \
  "SELECT vehicle_id, latitude, longitude, timestamp FROM vehicle_loctions ORDER BY id DESC LIMIT 10;"
```

**3. API.** Use the curl commands above or the Postman collection.

**4. Geofence → RabbitMQ.**

```bash
docker compose logs -f api worker | grep -i geofence
docker compose exec postgres psql -U fleet -d fleet -c "SELECT * FROM geofence_events ORDER BY id DESC LIMIT 5;"
```

The **Geofence events** panel on the dashboard shows the same data.

You can also watch the traffic in the RabbitMQ UI at http://localhost:15672 (login `fleet` / `fleet`): open **Queues → geofence_alerts** and **Exchanges → fleet.events**. The worker acknowledges messages immediately, so the queue normally looks empty. To see messages pile up in the queue, run `docker compose stop worker`, wait about a minute, and then use **Get messages** in the UI. Run `docker compose start worker` to resume consuming.

### Manual MQTT test

> **Windows Git Bash:** run `export MSYS_NO_PATHCONV=1` first. Otherwise Git Bash rewrites topics such as `/fleet/...` into Windows paths. PowerShell and Linux/macOS shells don't need this.

Send one reading yourself, for example right on the geofence center so it triggers an entry event:

```bash
docker compose exec mosquitto mosquitto_pub -t /fleet/vehicle/B9999TST/location \
  -m '{"vehicle_id":"B9999TST","latitude":-6.1754,"longitude":106.8272,"timestamp":1715003456}'
```

An invalid payload is rejected and logged in `docker compose logs api`:

```bash
docker compose exec mosquitto mosquitto_pub -t /fleet/vehicle/B9999TST/location \
  -m '{"vehicle_id":"B9999TST","latitude":999,"longitude":106.8,"timestamp":1715003456}'
```

## Configuration

All settings come from environment variables, and their defaults are set in [docker-compose.yml](docker-compose.yml).

| Variable | Service | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | api, worker | — (required) | PostgreSQL connection string |
| `MQTT_BROKER_URL` | api, publisher | `tcp://localhost:1883` | MQTT broker |
| `MQTT_TOPIC` | api | `/fleet/vehicle/+/location` | Subscription topic |
| `HTTP_ADDR` | api | `:8080` | HTTP listen address |
| `OAUTH_CLIENT_ID` / `OAUTH_CLIENT_SECRET` | api | — (required) | Client credentials for `/oauth/token` |
| `JWT_SECRET` / `TOKEN_TTL` | api | — (required) / `1h` | Token signing secret and lifetime |
| `RABBITMQ_URL` | api, worker | `amqp://guest:guest@localhost:5672/` | RabbitMQ connection |
| `RABBITMQ_EXCHANGE` / `RABBITMQ_QUEUE` / `RABBITMQ_ROUTING_KEY` | api, worker | `fleet.events` / `geofence_alerts` / `geofence.entry` | Messaging topology |
| `GEOFENCE_NAME` / `GEOFENCE_LAT` / `GEOFENCE_LON` / `GEOFENCE_RADIUS_M` | api | `Monas` / `-6.1754` / `106.8272` / `50` | Geofence point and radius |
| `VEHICLE_IDS` | publisher | `B1234XYZ,B5678ABC` | Comma-separated mock vehicles |
| `PUBLISH_INTERVAL` / `SPEED_MPS` | publisher | `2s` / `20` | Publish interval and simulated speed |

## Running without Docker (development)

You need Go 1.25+. Start the infrastructure with `docker compose up -d postgres mosquitto rabbitmq`, and then run:

```bash
go test ./...

# terminal 1
DATABASE_URL=postgres://fleet:fleet@localhost:5432/fleet?sslmode=disable \
RABBITMQ_URL=amqp://fleet:fleet@localhost:5672/ \
OAUTH_CLIENT_ID=fleet-client OAUTH_CLIENT_SECRET=fleet-secret JWT_SECRET=dev \
go run ./cmd/api

# terminal 2
DATABASE_URL=postgres://fleet:fleet@localhost:5432/fleet?sslmode=disable \
RABBITMQ_URL=amqp://fleet:fleet@localhost:5672/ go run ./cmd/worker

# terminal 3
go run ./cmd/publisher
```

## Project structure

```
cmd/
  api/          API service: MQTT subscriber, ingestion, geofence, REST API
  worker/       geofence_alerts consumer (logs + stores events)
  publisher/    mock MQTT publisher (every 2 s)
internal/
  api/          Gin router, handlers, auth middleware
  auth/         OAuth2 client credentials + JWT
  config/       environment configuration
  geofence/     haversine distance + entry detection
  messaging/    RabbitMQ publisher (confirms) and consumer
  model/        domain types + payload validation
  mqttclient/   MQTT subscriber
  repository/   PostgreSQL access + migrations runner
  service/      business logic (ingest pipeline, queries)
migrations/     SQL schema (embedded into the api binary)
web/            single-page dashboard (embedded, served at /)
mosquitto/      broker config
postman/        Postman collection
Dockerfile      multi-stage build; SERVICE build arg picks api | worker | publisher
docker-compose.yml
```
