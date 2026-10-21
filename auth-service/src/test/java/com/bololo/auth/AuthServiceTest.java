package com.bololo.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.argThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import java.util.Optional;
import java.util.UUID;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;

class AuthServiceTest {
    private AccountRepository accounts;
    private LobbySessionProvider lobbySessions;
    private AuthService service;

    @BeforeEach
    void setUp() {
        accounts = mock(AccountRepository.class);
        lobbySessions = mock(LobbySessionProvider.class);
        service = new AuthService(accounts, new BCryptPasswordEncoder(4), lobbySessions);
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
