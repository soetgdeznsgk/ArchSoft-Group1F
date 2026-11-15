# Auth Service

Java 21 / Spring Boot service that issues every player session (guests and accounts) and manages accounts, secured with Spring Security. It owns account credentials and its PostgreSQL database, and it does not depend on any other service.

## API

| Route | Access | Description |
|---|---|---|
| `POST /auth/guest` | Public | `{"nickname":"Ana"}`; returns `201 {player, token}`, a guest session without password |
| `POST /auth/register` | Public | `{"username":"ana","password":"a-long-password"}`; creates an account and returns `{player, token, account}` |
| `POST /auth/login` | Public | Same request and response; returns a session for the existing account |
| `GET /auth/me` | Account session | Returns `{id, username}` |
| `POST /auth/password` | Account session | `{"currentPassword":"...","newPassword":"..."}`; responds `204` |
| `GET /healthz` | Public | Container health check; Traefik does not route it |

Any other route is denied. Errors use the Lobby's `{code, message}` shape, with messages in Spanish so the client can show them as they are.

Usernames are case-insensitive and must be 3-20 characters using letters, numbers, `.`, `_`, or `-`. Passwords require 8-72 UTF-8 bytes and are stored as BCrypt hashes. Failed logins use the same response and the same BCrypt work whether the username is missing or the password is incorrect.

Five consecutive failures lock the account for one minute: login responds `429 too_many_attempts` with `Retry-After`, even with the correct password.

Guest nicknames are trimmed, inner spaces collapsed, and must have 1-20 characters (`422 invalid_nickname` otherwise).

## Session tokens

`SessionTokens` signs every session with RS256 and the session private key, which only Auth receives. Claims: `sub` (player id), `scope: session`, `nickname`, `iat`, `exp` (12 hours) and, for accounts only, `accountId`. A guest gets a random player id; an account always gets the same one (`p_` plus its UUID without dashes), so every login reaches the Lobby as the same player.

The Lobby Manager verifies sessions with the session public key and creates the player the first time it sees one. It still issues game tokens, signed with its own key pair; Auth never sees that key.

## Security model

`SecurityConfig` runs Spring Security as a stateless OAuth2 resource server. Protected routes require `Authorization: Bearer <session token>`, verified with the session public key and RS256 only: tokens signed with another key or algorithm (for example HS256 using the public key as secret) are refused, and so are game tokens. A token must carry `scope: session` and an `accountId` claim, so guest sessions get `403 account_required`.

Session tokens are stateless, so changing the password does not revoke the ones already issued: they expire on their own after 12 hours.

## Run and test

From the repository root, configure the `AUTH_POSTGRES_*` values in `.env`, then run:

```powershell
./scripts/gen-jwt-keys.sh
docker compose up --build --wait auth auth-postgres
```

Run service tests with:

```powershell
mvn -f auth-service/pom.xml test
```

Auth reads `SESSION_PRIVATE_KEY_FILE` and `SESSION_PUBLIC_KEY_FILE` (PEM files that `scripts/gen-jwt-keys.sh` creates under `secrets/jwt/` and Compose mounts as secrets). The tests generate their own key pairs. Keep the keys and the database password out of source control.
