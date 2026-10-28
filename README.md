# data-finder: local development

This repository owns its Compose environment and local tooling. No sibling checkout is
required. The default dependency inventory is **rabbitmq**. The `app` profile
builds this checkout. Broker, database and Keycloak data use project-scoped named volumes.

## Prerequisites and settings

Install Docker Engine with Compose v2.24.4+ (including `!reset` support for acceptance),
Make and Python 3. Host Go commands require the version in `go.mod`; Node services require
Node 22 and their declared npm packages. Renderer workflows also require
`npx playwright install --with-deps chromium` on the host. Authenticate Docker to GHCR if
your selected packages are private; registry tokens stay in Docker's credential store.
Copy `.env.example` to `.env` and replace local password placeholders. Use URL-safe local
password characters or percent-encode credentials in connection URLs. Keep secrets out of
Git and logs. These local credentials are externally supplied and are not demo user accounts.

For each setting: Make command-line > exported shell > selected `ENV_FILE` > Compose default.
Empty dependency tags and owners are errors. `GHCR_OWNER` defaults in the example to
`ravilock`; project image references themselves require a nonempty owner. Override a sibling
with its `<SERVICE>_IMAGE_TAG`, such as `CRAWLER_IMAGE_TAG`. Normal `make config` prints
only service names and image references. Full `docker compose config` is a sensitive
operation and can print credentials. `COMPOSE_PROJECT_NAME` defaults to `price-data-finder-local`;
use a distinct value and host ports for simultaneous environments.

## Run dependencies, then the checkout

```sh
cp .env.example .env
# Edit .env to supply local-only credentials and desired host ports.
make config
make deps.up
# Equivalently, after reviewing image access: docker compose up -d
make run
```

`make deps.up` pulls selected dependencies and stops on failure; it never substitutes a
cached image after a failed pull. The direct Compose command uses its normal pull policy;
explicitly pull first when freshness/availability verification is required.
This service does not own a database and has no local migration command.
For Node services, run `npm install` before host execution. `make run` maps database/broker
connections to the configured localhost ports, and binds the host process to `127.0.0.1`.

To build and run this checkout in a container:

```sh
make app.up
# Equivalent app-profile source build after dependency pull:
docker compose --profile app up --build -d
```

Container processes use Compose names and internal ports; the host process uses localhost.
Default application host port is `19180`, AMQP is `19172`, management is `19173`,
and owned PostgreSQL is `19132` when present. Keycloak uses `19181` when selected;
a sibling user API uses `19182`. Ports bind only to loopback. Configure `APP_PORT`,
`RABBITMQ_PORT`, `RABBITMQ_MANAGEMENT_PORT`, `POSTGRES_PORT`, `KEYCLOAK_PORT`, and
`USER_API_PORT` in `.env`; sibling database port overrides are named in Compose.

The browser-visible issuer is `OIDC_BASE_URL/realms/price-intelligence`. User API verifies
that same issuer and fetches keys through `http://keycloak:8080` inside Compose; host
`make run` maps this key URL to the published Keycloak port. If browser origins change,
update this repository's `local/keycloak/price-intelligence-realm.json` redirect/web origins
before importing a fresh realm, or update the retained realm through the local admin UI.
An existing realm import does not overwrite retained Keycloak settings.

## Optional workflows

Enable profiles only for the workflow being exercised, together with the checkout's `app`
profile for container execution. Their complete additional dependency closure is:

| Profile | Additional dependencies |
|---|---|
| collection | crawler, crawler-db |

```sh
# Example: select one profile listed above, without requiring sibling source.
make deps.up WORKFLOW_PROFILES="collection"
make app.up WORKFLOW_PROFILES="collection"
# Or docker compose --profile collection --profile app up --build -d
```

The controlled mock-source image/profile is delivered separately by T120 after its real
publication; no static fixture or invented image tag is substituted here. Dependency-only
startup is distinct from a working collection, browser, monitoring or demo workflow.

## Image inventory and compatibility

Official providers: Docker Official Images for `postgres:16-alpine` (PostgreSQL) and
`rabbitmq:3.13-management-alpine` (RabbitMQ); Keycloak's official
`quay.io/keycloak/keycloak:26.1.4` when identity is selected. Only the dependencies present
in this Compose file are started. These explicit infrastructure tags were checked against
the registry on 2026-10-27. Project sibling defaults, also verified available on that date:

- `ghcr.io/ravilock/price-crawler-crawler:31c8ff5c57be3921c41b9d7662b498623199861c`

These are published baseline commits using v1 messaging/configuration conventions. No
integrated readiness/business-workflow validation is claimed for this selection. `make`
startup warns that compatibility has not been established. An override is permitted, but
must remain a published explicit version or commit tag; changing a tag does not establish
content compatibility. Missing tags fail visibly. Review and record actual image content
and acceptance evidence before claiming compatibility.

## Source changes, verification and shutdown

For a reversible source-only check, change one startup log message in
`cmd/data-finder/main.go`
(keep behavior unchanged).
Stop the host process with Ctrl-C and run `make run` again to observe the new message
in the terminal. For a container, run
`docker compose --profile app up --build -d app` and inspect `docker compose logs app`.
Revert the source edit and repeat.
Go/Node container assets rebuild in the Dockerfile; the frontend host dev server rebuilds
assets automatically. No migration is needed for this log/title edit.
Run the repository's `make test lint build` checks before publishing changes.

```sh
make down
# Or docker compose --profile '*' down
```

Normal shutdown retains volumes. Back up any retained data before an explicit reset,
verify the selected project with `docker compose ls`, then deliberately select
`docker compose --profile '*' down --volumes` only for that disposable project.
Never reset automatically after startup or migration errors. Dirty-version recovery must
stop the owner, inspect its version/error and actual schema, back up data, and repair or
restore the schema before an operator explicitly corrects the version and retries; no
automatic rollback or forced version is permitted.

## Delivery status

The process exposes `/healthz` for liveness and `/readyz` for an authenticated
RabbitMQ connection. It waits up to `READINESS_TIMEOUT_SECONDS` (default 120)
for the initial connection, then reports degraded readiness during disconnection
and reconnects using exponential delays bounded by
`RECONNECT_INITIAL_DELAY_SECONDS` (default 1) and `RECONNECT_MAX_DELAY_SECONDS`
(default 30). The app-profile health check uses `/readyz`; readiness does not
assert that a collection workflow or its future consumers are available.

Infrastructure dependencies are health-gated as applicable. The checked-out app's
health endpoints establish only the dependencies described above; they do not establish
business workflow or integrated demo readiness. After catalog, alerts, or broker
recovery, recurring group monitoring must wait for the synchronized demand watermark
before its next recovered scheduling decision. That later workflow synchronization is
not represented by process health and remains subject to its own acceptance checks.

## Verification — 2026-10-27

`make test lint build`, Compose resolution and the dependency-image inventory check
passed. The current test target contains no service behavior tests yet. Minimum
dependencies passed real `docker compose up --wait` in isolated project `local-check-data-finder-201fb811`,
with host ports removed; normal shutdown retained its named volumes. This verifies
dependency startup, not application, migration, demo or business-workflow readiness.
