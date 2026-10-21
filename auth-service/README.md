# Auth Service

Java 21 / Spring Boot service for account registration and login. It owns account credentials and its PostgreSQL database. Guests continue to use the Lobby Manager's existing `POST /api/session` endpoint.

## API

- `POST /auth/register` — `{"username":"ana","password":"a-long-password"}`; creates an account and returns the Lobby-compatible `{player, token}` session.
- `POST /auth/login` — same request shape; returns a session for the existing account.
- `GET /healthz` — container health check.

Usernames are case-insensitive and must be 3-20 characters using letters, numbers, `.`, `_`, or `-`. Passwords require 8-72 UTF-8 bytes and are stored as BCrypt hashes. Failed logins use the same response whether the username is missing or the password is incorrect.

After credentials are verified, Auth calls `POST /internal/auth/session` on the Lobby Manager over Compose's private network with `X-Service-Token`. The Lobby Manager idempotently maps the Auth account UUID to a player row and issues its existing session JWT. Auth never signs player or game tokens itself.

## Run and test

From the repository root, configure the `AUTH_POSTGRES_*` values in `.env`, then run:

```powershell
docker compose up --build --wait auth auth-postgres lobby-manager postgres
```

Run service tests with:

```powershell
mvn -f auth-service/pom.xml test
```

The Auth service requires the existing `SERVICE_SECRET` to match the Lobby Manager's value. Keep this secret and the database password out of source control.
