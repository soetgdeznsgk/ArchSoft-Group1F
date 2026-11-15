package com.bololo.auth;

import static org.assertj.core.api.Assertions.assertThat;

import java.nio.charset.StandardCharsets;
import java.security.KeyPair;
import java.time.Instant;
import java.util.Date;
import java.util.Map;
import java.util.UUID;

import com.nimbusds.jose.JOSEException;
import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.crypto.MACSigner;
import com.nimbusds.jose.crypto.RSASSASigner;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.HttpEntity;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpMethod;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;

@SpringBootTest(
        webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {
            "spring.datasource.url=jdbc:h2:mem:auth;MODE=PostgreSQL;DB_CLOSE_DELAY=-1",
            "spring.datasource.username=sa",
            "spring.datasource.password=",
            "spring.datasource.driver-class-name=org.h2.Driver"
        })
class AuthApiIntegrationTest {
    @Autowired
    private TestRestTemplate http;

    @Autowired
    private JdbcTemplate jdbc;

    @DynamicPropertySource
    static void sessionKeys(DynamicPropertyRegistry properties) {
        properties.add("SESSION_PRIVATE_KEY_FILE", TestKeys.SESSION_PRIVATE_FILE::toString);
        properties.add("SESSION_PUBLIC_KEY_FILE", TestKeys.SESSION_PUBLIC_FILE::toString);
    }

    @BeforeEach
    void resetDatabase() {
        jdbc.update("DELETE FROM accounts");
    }

    @Test
    void guestSessionWorksWithoutAccountAndCannotUseAccountRoutes() {
        ResponseEntity<Map<String, Object>> guest = post("/auth/guest", Map.of("nickname", "Ana"));

        assertThat(guest.getStatusCode()).isEqualTo(HttpStatus.CREATED);
        assertThat(guest.getBody()).containsKeys("player", "token").doesNotContainKey("account");
        ResponseEntity<Map<String, Object>> me = get("/auth/me", guest.getBody().get("token").toString());
        assertThat(me.getStatusCode()).isEqualTo(HttpStatus.FORBIDDEN);
        assertThat(me.getBody()).containsEntry("code", "account_required");

        ResponseEntity<Map<String, Object>> blank = post("/auth/guest", Map.of("nickname", "   "));
        assertThat(blank.getStatusCode()).isEqualTo(HttpStatus.UNPROCESSABLE_ENTITY);
        assertThat(blank.getBody()).containsEntry("code", "invalid_nickname");
    }

    @Test
    void registrationAndLoginReturnTheSamePlayerAndAUsableToken() {
        Map<String, String> request = Map.of("username", "Ana", "password", "safe-pass-123");

        ResponseEntity<Map<String, Object>> registered = post("/auth/register", request);
        ResponseEntity<Map<String, Object>> loggedIn = post("/auth/login", request);

        assertThat(registered.getStatusCode()).isEqualTo(HttpStatus.CREATED);
        assertThat(loggedIn.getStatusCode()).isEqualTo(HttpStatus.OK);
        Map<String, Object> registrationBody = registered.getBody();
        Map<String, Object> loginBody = loggedIn.getBody();
        assertThat(registrationBody).containsKeys("player", "token", "account");
        assertThat(registrationBody.get("player")).isEqualTo(loginBody.get("player"));
        ResponseEntity<Map<String, Object>> me = get("/auth/me", loginBody.get("token").toString());
        assertThat(me.getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(me.getBody()).containsEntry("username", "ana");
    }

    @Test
    void duplicateRegistrationAndInvalidLoginHaveSafeErrors() {
        Map<String, String> request = Map.of("username", "ana", "password", "safe-pass-123");
        post("/auth/register", request);

        ResponseEntity<Map<String, Object>> duplicate = post("/auth/register", request);
        ResponseEntity<Map<String, Object>> invalidLogin = post(
                "/auth/login",
                Map.of("username", "ana", "password", "incorrect-password"));

        assertThat(duplicate.getStatusCode()).isEqualTo(HttpStatus.CONFLICT);
        assertThat(duplicate.getBody()).containsEntry("code", "username_taken");
        assertThat(invalidLogin.getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
        assertThat(invalidLogin.getBody()).containsEntry("code", "invalid_credentials");
    }

    @Test
    void registrationRejectsPasswordsLongerThanBcryptLimit() {
        ResponseEntity<Map<String, Object>> response = post(
                "/auth/register",
                Map.of("username", "ana", "password", "a".repeat(73)));

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.UNPROCESSABLE_ENTITY);
        assertThat(response.getBody()).containsEntry("code", "invalid_password");
    }

    @Test
    void loginReturnsTheAccountAndMeReadsItFromTheSessionToken() {
        Map<String, Object> account = accountOf(post(
                "/auth/register", Map.of("username", "ana", "password", "safe-pass-123")));

        ResponseEntity<Map<String, Object>> me = get("/auth/me", sessionToken(account.get("id").toString(), 3600));

        assertThat(me.getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(me.getBody()).containsEntry("id", account.get("id")).containsEntry("username", "ana");
    }

    @Test
    void protectedEndpointsRejectMissingForgedAndExpiredTokens() {
        String id = UUID.randomUUID().toString();

        assertThat(get("/auth/me", null).getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
        assertThat(get("/auth/me", "not-a-jwt").getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
        assertThat(get("/auth/me", sessionToken(id, -60)).getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
        assertThat(get("/auth/me", token(claims(id, "session", 3600), TestKeys.OTHER)).getStatusCode())
                .isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    @Test
    void onlyRs256WithTheSessionKeyIsAccepted() {
        JWTClaimsSet session = claims(UUID.randomUUID().toString(), "session", 3600);
        String publicKeyAsHmacSecret = new String(TestKeys.SESSION.getPublic().getEncoded(), StandardCharsets.ISO_8859_1);

        assertThat(get("/auth/me", hmacToken(session, publicKeyAsHmacSecret)).getStatusCode())
                .isEqualTo(HttpStatus.UNAUTHORIZED);
        assertThat(get("/auth/me", hmacToken(session, "old-shared-jwt-secret-with-at-least-32-bytes"))
                .getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    @Test
    void tokensWithoutSessionScopeAndGuestSessionsAreForbidden() {
        JWTClaimsSet withoutScope = claims(UUID.randomUUID().toString(), null, 3600);
        JWTClaimsSet guest = claims(null, "session", 3600);

        assertThat(get("/auth/me", token(withoutScope, TestKeys.SESSION)).getStatusCode())
                .isEqualTo(HttpStatus.FORBIDDEN);
        ResponseEntity<Map<String, Object>> response = get("/auth/me", token(guest, TestKeys.SESSION));
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.FORBIDDEN);
        assertThat(response.getBody()).containsEntry("code", "account_required");
    }

    @Test
    void changingThePasswordReplacesTheOldOne() {
        Map<String, Object> account = accountOf(post(
                "/auth/register", Map.of("username", "ana", "password", "safe-pass-123")));
        String token = sessionToken(account.get("id").toString(), 3600);

        ResponseEntity<Map<String, Object>> wrong = post(
                "/auth/password", Map.of("currentPassword", "not-the-password", "newPassword", "new-pass-456"), token);
        ResponseEntity<Map<String, Object>> changed = post(
                "/auth/password", Map.of("currentPassword", "safe-pass-123", "newPassword", "new-pass-456"), token);

        assertThat(wrong.getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
        assertThat(changed.getStatusCode()).isEqualTo(HttpStatus.NO_CONTENT);
        assertThat(post("/auth/login", Map.of("username", "ana", "password", "safe-pass-123")).getStatusCode())
                .isEqualTo(HttpStatus.UNAUTHORIZED);
        assertThat(post("/auth/login", Map.of("username", "ana", "password", "new-pass-456")).getStatusCode())
                .isEqualTo(HttpStatus.OK);
    }

    @Test
    void repeatedFailuresLockTheAccountTemporarily() {
        post("/auth/register", Map.of("username", "ana", "password", "safe-pass-123"));
        Map<String, String> wrong = Map.of("username", "ana", "password", "incorrect-password");

        for (int attempt = 0; attempt < AuthService.MAX_FAILED_LOGINS; attempt++) {
            assertThat(post("/auth/login", wrong).getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
        }
        ResponseEntity<Map<String, Object>> locked = post(
                "/auth/login", Map.of("username", "ana", "password", "safe-pass-123"));

        assertThat(locked.getStatusCode()).isEqualTo(HttpStatus.TOO_MANY_REQUESTS);
        assertThat(locked.getBody()).containsEntry("code", "too_many_attempts");
        assertThat(locked.getHeaders().getFirst(HttpHeaders.RETRY_AFTER)).isNotBlank();

        jdbc.update("UPDATE accounts SET locked_until = now() - INTERVAL '1' SECOND");
        assertThat(post("/auth/login", Map.of("username", "ana", "password", "safe-pass-123")).getStatusCode())
                .isEqualTo(HttpStatus.OK);
    }

    @Test
    void unknownRoutesAreDeniedAndHealthStaysPublic() {
        assertThat(get("/healthz", null).getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(get("/auth/accounts", null).getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    @Test
    void malformedJsonIsARequestErrorNotAnAuthError() {
        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);

        ResponseEntity<String> response = http.exchange(
                "/auth/login", HttpMethod.POST, new HttpEntity<>("{not json", headers), String.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
    }

    private static String sessionToken(String accountId, long lifetimeSeconds) {
        return token(claims(accountId, "session", lifetimeSeconds), TestKeys.SESSION);
    }

    private static JWTClaimsSet claims(String accountId, String scope, long lifetimeSeconds) {
        JWTClaimsSet.Builder claims = new JWTClaimsSet.Builder()
                .subject("p_test")
                .expirationTime(Date.from(Instant.now().plusSeconds(lifetimeSeconds)));
        if (accountId != null) {
            claims.claim("accountId", accountId);
        }
        if (scope != null) {
            claims.claim("scope", scope);
        }
        return claims.build();
    }

    private static String token(JWTClaimsSet claims, KeyPair keys) {
        try {
            SignedJWT jwt = new SignedJWT(new JWSHeader(JWSAlgorithm.RS256), claims);
            jwt.sign(new RSASSASigner(keys.getPrivate()));
            return jwt.serialize();
        } catch (JOSEException failure) {
            throw new IllegalStateException(failure);
        }
    }

    private static String hmacToken(JWTClaimsSet claims, String secret) {
        try {
            SignedJWT jwt = new SignedJWT(new JWSHeader(JWSAlgorithm.HS256), claims);
            jwt.sign(new MACSigner(secret.getBytes(StandardCharsets.ISO_8859_1)));
            return jwt.serialize();
        } catch (JOSEException failure) {
            throw new IllegalStateException(failure);
        }
    }

    @SuppressWarnings("unchecked")
    private static Map<String, Object> accountOf(ResponseEntity<Map<String, Object>> session) {
        return (Map<String, Object>) session.getBody().get("account");
    }

    private ResponseEntity<Map<String, Object>> get(String path, String bearer) {
        return send(path, HttpMethod.GET, null, bearer);
    }

    private ResponseEntity<Map<String, Object>> post(String path, Map<String, String> body) {
        return send(path, HttpMethod.POST, body, null);
    }

    private ResponseEntity<Map<String, Object>> post(String path, Map<String, String> body, String bearer) {
        return send(path, HttpMethod.POST, body, bearer);
    }

    private ResponseEntity<Map<String, Object>> send(
            String path, HttpMethod method, Map<String, String> body, String bearer) {
        HttpHeaders headers = new HttpHeaders();
        if (bearer != null) {
            headers.setBearerAuth(bearer);
        }
        return http.exchange(path, method, new HttpEntity<>(body, headers), new ParameterizedTypeReference<>() {});
    }
}
