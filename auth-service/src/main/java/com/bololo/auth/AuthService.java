package com.bololo.auth;

import java.nio.charset.StandardCharsets;
import java.util.Locale;
import java.util.UUID;

import org.springframework.dao.DuplicateKeyException;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;

@Service
class AuthService {
    private final AccountRepository accounts;
    private final PasswordEncoder passwordEncoder;
    private final LobbySessionProvider lobbySessions;

    AuthService(
            AccountRepository accounts,
            PasswordEncoder passwordEncoder,
            LobbySessionProvider lobbySessions) {
        this.accounts = accounts;
        this.passwordEncoder = passwordEncoder;
        this.lobbySessions = lobbySessions;
    }

    SessionResponse register(String usernameInput, String password) {
        String username = normalizeUsername(usernameInput);
        validatePassword(password);
        Account account = new Account(UUID.randomUUID(), username, passwordEncoder.encode(password));
        try {
            accounts.create(account);
        } catch (DuplicateKeyException duplicate) {
            throw new AuthException(409, "username_taken", "That username is already registered.");
        }
        return sessionFor(account);
    }

    SessionResponse login(String usernameInput, String password) {
        String username = normalizeUsername(usernameInput);
        if (!hasValidPasswordLength(password)) {
            throw invalidCredentials();
        }
        Account account = accounts.findByUsername(username).orElseThrow(AuthService::invalidCredentials);
        if (!passwordEncoder.matches(password, account.passwordHash())) {
            throw invalidCredentials();
        }
        return sessionFor(account);
    }

    private SessionResponse sessionFor(Account account) {
        LobbySessionResponse lobbySession = lobbySessions.createSession(account.id(), account.username());
        return new SessionResponse(lobbySession.player(), lobbySession.token());
    }

    private static String normalizeUsername(String username) {
        if (username == null || !username.matches("^[A-Za-z0-9_.-]{3,20}$")) {
            throw new AuthException(422, "invalid_username", "Username must be 3-20 letters, numbers, dots, dashes, or underscores.");
        }
        return username.toLowerCase(Locale.ROOT);
    }

    private static void validatePassword(String password) {
        if (!hasValidPasswordLength(password)) {
            throw new AuthException(422, "invalid_password", "Password must contain 8-72 UTF-8 bytes.");
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
        return new AuthException(401, "invalid_credentials", "Username or password is incorrect.");
    }
}
