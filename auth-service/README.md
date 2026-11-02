# Auth Service

Java 21 / Spring Boot service for account registration, login and account management, secured with Spring Security. It owns account credentials and its PostgreSQL database. Guests continue to use the Lobby Manager's existing `POST /api/session` endpoint.

## API

| Route | Access | Description |
|---|---|---|
| `POST /auth/register` | Public | `{"username":"ana","password":"a-long-password"}`; creates an account and returns `{player, token, account}`, where `player` and `token` are the Lobby-compatible session |
| `POST /auth/login` | Public | Same request and response; returns a session for the existing account |
| `GET /auth/me` | Account session | Returns `{id, username}` |
| `POST /auth/password` | Account session | `{"currentPassword":"...","newPassword":"..."}`; responds `204` |
| `GET /healthz` | Public | Container health check; Traefik does not route it |

Any other route is denied. Errors use the Lobby's `{code, message}` shape, with messages in Spanish so the client can show them as they are.

Usernames are case-insensitive and must be 3-20 characters using letters, numbers, `.`, `_`, or `-`. Passwords require 8-72 UTF-8 bytes and are stored as BCrypt hashes. Failed logins use the same response and the same BCrypt work whether the username is missing or the password is incorrect.

Five consecutive failures lock the account for one minute: login responds `429 too_many_attempts` with `Retry-After`, even with the correct password.

After credentials are verified, Auth calls `POST /internal/auth/session` on the Lobby Manager over Compose's private network with `X-Service-Token`. The Lobby Manager idempotently maps the Auth account UUID to a player row and issues its existing session JWT. Auth never signs player or game tokens itself.

## Security model

`SecurityConfig` runs Spring Security as a stateless OAuth2 resource server. Protected routes require `Authorization: Bearer <session token>`, validated with `JWT_SECRET` (HS256, at least 32 bytes), the same secret the Lobby Manager uses to sign. A token must carry `scope: session`, so game tokens are refused, and an `accountId` claim, so guest sessions get `403 account_required`.

Session tokens are stateless, so changing the password does not revoke the ones already issued: they expire on their own after 12 hours.

## Run and test

From the repository root, configure the `AUTH_POSTGRES_*` values in `.env`, then run:

```powershell
docker compose up --build --wait auth auth-postgres lobby-manager postgres
```

Run service tests with:

```powershell
mvn -f auth-service/pom.xml test
```

The Auth service requires `SERVICE_SECRET` and `JWT_SECRET` to match the Lobby Manager's values. Keep these secrets and the database password out of source control.
