package com.bololo.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.argThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.Optional;
import java.util.UUID;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;

class AuthServiceTest {
    private static final Instant NOW = Instant.parse("2026-11-01T12:00:00Z");

    private AccountRepository accounts;
    private LobbySessionProvider lobbySessions;
    private AuthService service;

    @BeforeEach
    void setUp() {
        accounts = mock(AccountRepository.class);
        lobbySessions = mock(LobbySessionProvider.class);
        service = new AuthService(
                accounts, new BCryptPasswordEncoder(4), lobbySessions, Clock.fixed(NOW, ZoneOffset.UTC));
    }

    @Test
    void registerHashesPasswordAndReturnsLobbySession() {
        PlayerResponse player = new PlayerResponse("p_123", "ana");
        when(lobbySessions.createSession(any(), any()))
                .thenReturn(new LobbySessionResponse(player, "session-token"));

        SessionResponse response = service.register("Ana", "safe-pass-123");

        assertThat(response.player()).isEqualTo(player);
        assertThat(response.token()).isEqualTo("session-token");
        verify(accounts).create(argThat(account -> {
            assertThat(account.id()).isNotNull();
            assertThat(account.username()).isEqualTo("ana");
            assertThat(account.passwordHash()).isNotEqualTo("safe-pass-123");
            assertThat(new BCryptPasswordEncoder(4).matches("safe-pass-123", account.passwordHash())).isTrue();
            return true;
        }));
        verify(lobbySessions).createSession(any(UUID.class), org.mockito.ArgumentMatchers.eq("ana"));
    }

    @Test
    void loginRejectsWrongPasswordWithoutProvisioningSession() {
        UUID accountId = UUID.randomUUID();
        String hash = new BCryptPasswordEncoder(4).encode("correct-pass");
        when(accounts.findByUsername("ana"))
                .thenReturn(Optional.of(new Account(accountId, "ana", hash)));

        assertThatThrownBy(() -> service.login("ANA", "incorrect"))
                .isInstanceOfSatisfying(AuthException.class, exception -> {
                    assertThat(exception.status()).isEqualTo(401);
                    assertThat(exception.code()).isEqualTo("invalid_credentials");
                });
        verify(lobbySessions, never()).createSession(any(), any());
        verify(accounts).recordFailedLogin(
                accountId, AuthService.MAX_FAILED_LOGINS, NOW.plus(AuthService.LOCK_DURATION));
    }

    @Test
    void lockedAccountRejectsEvenTheCorrectPassword() {
        UUID accountId = UUID.randomUUID();
        String hash = new BCryptPasswordEncoder(4).encode("correct-pass");
        when(accounts.findByUsername("ana"))
                .thenReturn(Optional.of(new Account(accountId, "ana", hash, NOW.plus(Duration.ofSeconds(30)))));

        assertThatThrownBy(() -> service.login("ana", "correct-pass"))
                .isInstanceOfSatisfying(AuthException.class, exception -> {
                    assertThat(exception.status()).isEqualTo(429);
                    assertThat(exception.code()).isEqualTo("too_many_attempts");
                    assertThat(exception.retryAfterSeconds()).isEqualTo(31);
                });
        verify(lobbySessions, never()).createSession(any(), any());
    }

    @Test
    void expiredLockAllowsLoginAndClearsFailures() {
        UUID accountId = UUID.randomUUID();
        String hash = new BCryptPasswordEncoder(4).encode("correct-pass");
        when(accounts.findByUsername("ana"))
                .thenReturn(Optional.of(new Account(accountId, "ana", hash, NOW.minusSeconds(1))));
        when(lobbySessions.createSession(any(), any()))
                .thenReturn(new LobbySessionResponse(new PlayerResponse("p_1", "ana"), "session-token"));

        SessionResponse response = service.login("ana", "correct-pass");

        assertThat(response.account().id()).isEqualTo(accountId.toString());
        verify(accounts).clearFailedLogins(accountId);
    }

    @Test
    void unknownUsernameIsIndistinguishableFromWrongPassword() {
        when(accounts.findByUsername("nadie")).thenReturn(Optional.empty());

        assertThatThrownBy(() -> service.login("nadie", "whatever-pass"))
                .isInstanceOfSatisfying(AuthException.class, exception -> {
                    assertThat(exception.status()).isEqualTo(401);
                    assertThat(exception.code()).isEqualTo("invalid_credentials");
                });
    }

    @Test
    void duplicateUsernameProducesConflict() {
        when(accounts.findByUsername("ana")).thenReturn(Optional.empty());
        org.mockito.Mockito.doThrow(new DuplicateKeyException("duplicate"))
                .when(accounts).create(any());

        assertThatThrownBy(() -> service.register("ana", "safe-pass-123"))
                .isInstanceOfSatisfying(AuthException.class, exception -> {
                    assertThat(exception.status()).isEqualTo(409);
                    assertThat(exception.code()).isEqualTo("username_taken");
                });
    }
}
