package com.bololo.auth;

import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.Locale;
import java.util.Optional;
import java.util.UUID;

import org.springframework.dao.DuplicateKeyException;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;

@Service
class AuthService {
    static final int MAX_FAILED_LOGINS = 5;
    static final Duration LOCK_DURATION = Duration.ofMinutes(1);

    private final AccountRepository accounts;
    private final PasswordEncoder passwordEncoder;
    private final LobbySessionProvider lobbySessions;
    private final Clock clock;
    private final String decoyHash;

    AuthService(
            AccountRepository accounts,
            PasswordEncoder passwordEncoder,
            LobbySessionProvider lobbySessions,
            Clock clock) {
        this.accounts = accounts;
        this.passwordEncoder = passwordEncoder;
        this.lobbySessions = lobbySessions;
        this.clock = clock;
        this.decoyHash = passwordEncoder.encode(UUID.randomUUID().toString());
    }

    SessionResponse register(String usernameInput, String password) {
        String username = normalizeUsername(usernameInput);
        validatePassword(password);
        Account account = new Account(UUID.randomUUID(), username, passwordEncoder.encode(password));
        try {
            accounts.create(account);
        } catch (DuplicateKeyException duplicate) {
            throw new AuthException(409, "username_taken", "Ese nombre de usuario ya está registrado.");
        }
        return sessionFor(account);
    }

    SessionResponse login(String usernameInput, String password) {
        String username = normalizeUsername(usernameInput);
        if (!hasValidPasswordLength(password)) {
            throw invalidCredentials();
        }
        Optional<Account> found = accounts.findByUsername(username);
        if (found.isEmpty()) {
            // Same BCrypt work as a real account, so response time does not reveal which usernames exist.
            passwordEncoder.matches(password, decoyHash);
            throw invalidCredentials();
        }
        Account account = found.get();
        verifyPassword(account, password);
        accounts.clearFailedLogins(account.id());
        return sessionFor(account);
    }

    AccountResponse account(UUID accountId) {
        Account account = accounts.findById(accountId).orElseThrow(AuthService::accountRequired);
        return new AccountResponse(account.id().toString(), account.username());
    }

    void changePassword(UUID accountId, String currentPassword, String newPassword) {
        validatePassword(newPassword);
        Account account = accounts.findById(accountId).orElseThrow(AuthService::accountRequired);
        if (!hasValidPasswordLength(currentPassword)) {
            throw invalidCredentials();
        }
        verifyPassword(account, currentPassword);
        accounts.updatePassword(account.id(), passwordEncoder.encode(newPassword));
        accounts.clearFailedLogins(account.id());
    }

    static AuthException accountRequired() {
        return new AuthException(403, "account_required", "Esta acción requiere una cuenta registrada.");
    }

    private void verifyPassword(Account account, String password) {
        Instant now = clock.instant();
        if (account.lockedUntil() != null && account.lockedUntil().isAfter(now)) {
            long seconds = Duration.between(now, account.lockedUntil()).toSeconds() + 1;
            throw new AuthException(
                    429, "too_many_attempts", "Demasiados intentos fallidos. Espera un momento e inténtalo de nuevo.", seconds);
        }
        if (!passwordEncoder.matches(password, account.passwordHash())) {
            accounts.recordFailedLogin(account.id(), MAX_FAILED_LOGINS, now.plus(LOCK_DURATION));
            throw invalidCredentials();
        }
    }

    private SessionResponse sessionFor(Account account) {
        LobbySessionResponse lobbySession = lobbySessions.createSession(account.id(), account.username());
        return new SessionResponse(
                lobbySession.player(),
                lobbySession.token(),
                new AccountResponse(account.id().toString(), account.username()));
    }

    private static String normalizeUsername(String username) {
        if (username == null || !username.matches("^[A-Za-z0-9_.-]{3,20}$")) {
            throw new AuthException(422, "invalid_username", "El usuario debe tener de 3 a 20 letras, números, puntos, guiones o guiones bajos.");
        }
        return username.toLowerCase(Locale.ROOT);
    }

    private static void validatePassword(String password) {
        if (!hasValidPasswordLength(password)) {
            throw new AuthException(422, "invalid_password", "La contraseña debe tener entre 8 y 72 bytes.");
        }
    }

    private static boolean hasValidPasswordLength(String password) {
        if (password == null || password.isBlank()) {
            return false;
        }
        int bytes = password.getBytes(StandardCharsets.UTF_8).length;
        return bytes >= 8 && bytes <= 72;
    }

    private static AuthException invalidCredentials() {
        return new AuthException(401, "invalid_credentials", "Usuario o contraseña incorrectos.");
    }
}
