package com.bololo.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.when;

import java.util.Map;
import java.util.UUID;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.HttpEntity;
import org.springframework.http.HttpMethod;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.test.context.bean.override.mockito.MockitoBean;

@SpringBootTest(
        webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {
            "spring.datasource.url=jdbc:h2:mem:auth;MODE=PostgreSQL;DB_CLOSE_DELAY=-1",
            "spring.datasource.username=sa",
            "spring.datasource.password=",
            "spring.datasource.driver-class-name=org.h2.Driver",
            "SERVICE_SECRET=test-service-secret"
        })
class AuthApiIntegrationTest {
    @Autowired
    private TestRestTemplate http;

    @Autowired
    private JdbcTemplate jdbc;

    @MockitoBean
    private LobbySessionProvider lobbySessions;

    @BeforeEach
    void resetDatabaseAndLobby() {
        jdbc.update("DELETE FROM accounts");
        when(lobbySessions.createSession(any(UUID.class), anyString()))
                .thenAnswer(invocation -> {
                    UUID accountId = invocation.getArgument(0);
                    String nickname = invocation.getArgument(1);
                    return new LobbySessionResponse(
                            new PlayerResponse("p_" + accountId, nickname),
                            "lobby-token-" + accountId);
                });
    }

    @Test
    void registrationAndLoginReturnLobbyCompatibleSessions() {
        Map<String, String> request = Map.of("username", "Ana", "password", "safe-pass-123");

        ResponseEntity<Map<String, Object>> registered = post("/auth/register", request);
        ResponseEntity<Map<String, Object>> loggedIn = post("/auth/login", request);

        assertThat(registered.getStatusCode()).isEqualTo(HttpStatus.CREATED);
        assertThat(loggedIn.getStatusCode()).isEqualTo(HttpStatus.OK);
        Map<String, Object> registrationBody = registered.getBody();
        Map<String, Object> loginBody = loggedIn.getBody();
        assertThat(registrationBody).containsKeys("player", "token");
        assertThat(registrationBody.get("player")).isEqualTo(loginBody.get("player"));
        assertThat(registrationBody.get("token")).isEqualTo(loginBody.get("token"));
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

    private ResponseEntity<Map<String, Object>> post(String path, Map<String, String> body) {
        return http.exchange(
                path,
                HttpMethod.POST,
                new HttpEntity<>(body),
                new ParameterizedTypeReference<>() {});
    }
}
