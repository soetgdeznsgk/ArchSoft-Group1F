package com.bololo.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.argThat;
import static org.mockito.Mockito.mock;
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
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.security.oauth2.jose.jws.SignatureAlgorithm;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.security.oauth2.jwt.JwtDecoder;
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder;

class AuthServiceTest {
    private static final Instant NOW = Instant.parse("2026-11-01T12:00:00Z");

    private AccountRepository accounts;
    private AuthService service;
    private final JwtDecoder decoder = NimbusJwtDecoder.withPublicKey(TestKeys.publicKey(TestKeys.SESSION))
            .signatureAlgorithm(SignatureAlgorithm.RS256)
            .build();

    @BeforeEach
    void setUp() {
        accounts = mock(AccountRepository.class);
        SessionTokens tokens = new SessionTokens(
                TestKeys.privateKey(TestKeys.SESSION),
                TestKeys.publicKey(TestKeys.SESSION),
                Duration.ofHours(12),
                Clock.systemUTC());
        service = new AuthService(accounts, new BCryptPasswordEncoder(4), tokens, Clock.fixed(NOW, ZoneOffset.UTC));
    }

    @Test
    void registerHashesPasswordAndSignsAnAccountSession() {
        SessionResponse response = service.register("Ana", "safe-pass-123");

        verify(accounts).create(argThat(account -> {
            assertThat(account.id()).isNotNull();
            assertThat(account.username()).isEqualTo("ana");
            assertThat(account.passwordHash()).isNotEqualTo("safe-pass-123");
            assertThat(new BCryptPasswordEncoder(4).matches("safe-pass-123", account.passwordHash())).isTrue();
            return true;
        }));
        String accountId = response.account().id();
        assertThat(response.player().id()).isEqualTo("p_" + accountId.replace("-", ""));
        assertThat(response.player().nickname()).isEqualTo("ana");

        Jwt token = decoder.decode(response.token());
        assertThat(token.getHeaders()).containsEntry("alg", "RS256");
        assertThat(token.getSubject()).isEqualTo(response.player().id());
        assertThat(token.getClaimAsString("scope")).isEqualTo("session");
        assertThat(token.getClaimAsString("nickname")).isEqualTo("ana");
        assertThat(token.getClaimAsString("accountId")).isEqualTo(accountId);
        assertThat(token.getExpiresAt()).isAfter(token.getIssuedAt().plus(Duration.ofHours(11)));
    }

    @Test
    void guestSessionsHaveNoAccountAndADistinctPlayer() {
        SessionResponse first = service.guest("  Ana   María ");
        SessionResponse second = service.guest("Ana María");

        assertThat(first.account()).isNull();
        assertThat(first.player().nickname()).isEqualTo("Ana María");
        assertThat(first.player().id()).startsWith("p_").isNotEqualTo(second.player().id());
        Jwt token = decoder.decode(first.token());
        assertThat(token.getSubject()).isEqualTo(first.player().id());
        assertThat(token.getClaimAsString("scope")).isEqualTo("session");
        assertThat(token.getClaimAsString("nickname")).isEqualTo("Ana María");
        assertThat(token.hasClaim("accountId")).isFalse();
    }

    @ParameterizedTest
    @ValueSource(strings = {"", "    ", "xxxxxxxxxxxxxxxxxxxxx"})
    void guestRejectsBlankOrLongNicknames(String nickname) {
        assertThatThrownBy(() -> service.guest(nickname))
                .isInstanceOfSatisfying(AuthException.class, exception -> {
                    assertThat(exception.status()).isEqualTo(422);
                    assertThat(exception.code()).isEqualTo("invalid_nickname");
                });
    }

    @Test
    void accountPlayerIdIsStableAcrossLogins() {
        UUID accountId = UUID.randomUUID();
        String hash = new BCryptPasswordEncoder(4).encode("correct-pass");
        when(accounts.findByUsername("ana")).thenReturn(Optional.of(new Account(accountId, "ana", hash)));

        SessionResponse first = service.login("ana", "correct-pass");
        SessionResponse second = service.login("ANA", "correct-pass");

        assertThat(first.player()).isEqualTo(second.player());
        assertThat(first.player().id()).isEqualTo("p_" + accountId.toString().replace("-", ""));
    }

    @Test
    void loginRejectsWrongPassword() {
        UUID accountId = UUID.randomUUID();
        String hash = new BCryptPasswordEncoder(4).encode("correct-pass");
        when(accounts.findByUsername("ana"))
                .thenReturn(Optional.of(new Account(accountId, "ana", hash)));

        assertThatThrownBy(() -> service.login("ANA", "incorrect"))
                .isInstanceOfSatisfying(AuthException.class, exception -> {
                    assertThat(exception.status()).isEqualTo(401);
                    assertThat(exception.code()).isEqualTo("invalid_credentials");
                });
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
    }

    @Test
    void expiredLockAllowsLoginAndClearsFailures() {
        UUID accountId = UUID.randomUUID();
        String hash = new BCryptPasswordEncoder(4).encode("correct-pass");
        when(accounts.findByUsername("ana"))
                .thenReturn(Optional.of(new Account(accountId, "ana", hash, NOW.minusSeconds(1))));

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
