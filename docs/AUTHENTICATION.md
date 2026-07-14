# Authentication

## First-run setup

`GET /api/v1/auth/status` reports whether an administrator exists. Until setup is complete, `POST /api/v1/auth/setup` creates the first administrator. The endpoint stops accepting setup requests after that account exists.

Login uses `POST /api/v1/auth/login`. The current account is available at `GET /api/v1/auth/me`, and `POST /api/v1/auth/logout` invalidates the current session.

## Sessions

The server stores opaque session identifiers in SQLite and sends the identifier in an HTTP-only cookie. The cookie uses `SameSite=Lax`; set `SAMRAI_COOKIE_SECURE=true` when the public URL uses HTTPS.

Session lifetime is controlled by `SAMRAI_SESSION_DURATION`. Logging out removes the server-side session, not only the browser cookie.

The old PageTurner cookie name is accepted during the rename transition and migrated after a successful request.

## Passwords

Passwords are hashed with Argon2id. The encoded hash contains its own parameters, salt, and version, so changing the configured cost affects new password changes without invalidating existing accounts.

Administrators can create readers or administrators, disable accounts, reset passwords, and remove accounts. Users can change their own password after supplying the current one.

## Request protection

- Login attempts are rate limited by client and username.
- Authentication and administrative checks are enforced in server middleware.
- JSON handlers reject trailing objects after the expected request body.
- State-changing operations require the session cookie and use same-site browser behavior.
- Password hashes and raw session tokens are never returned by the API.

See [Users and permissions](USERS.md) for role behavior.
