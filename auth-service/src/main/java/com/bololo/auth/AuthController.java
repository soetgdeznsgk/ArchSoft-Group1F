package com.bololo.auth;

import java.util.UUID;

import jakarta.validation.Valid;

import org.springframework.http.HttpStatus;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

@RestController
class AuthController {
    private final AuthService authService;
    private final JdbcTemplate jdbc;

    AuthController(AuthService authService, JdbcTemplate jdbc) {
        this.authService = authService;
        this.jdbc = jdbc;
    }

    @PostMapping("/auth/register")
    @ResponseStatus(HttpStatus.CREATED)
    SessionResponse register(@Valid @RequestBody RegisterRequest request) {
        return authService.register(request.username(), request.password());
    }

    @PostMapping("/auth/login")
    SessionResponse login(@Valid @RequestBody CredentialsRequest request) {
        return authService.login(request.username(), request.password());
    }

    @GetMapping("/auth/me")
    AccountResponse me(@AuthenticationPrincipal Jwt session) {
        return authService.account(accountId(session));
    }

    @PostMapping("/auth/password")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    void changePassword(@AuthenticationPrincipal Jwt session, @Valid @RequestBody ChangePasswordRequest request) {
        authService.changePassword(accountId(session), request.currentPassword(), request.newPassword());
    }

    @GetMapping("/healthz")
    HealthResponse health() {
        jdbc.queryForObject("SELECT 1", Integer.class);
        return new HealthResponse("ok");
    }

    private static UUID accountId(Jwt session) {
        String accountId = session.getClaimAsString("accountId");
        if (accountId == null) {
            throw AuthService.accountRequired();
        }
        return UUID.fromString(accountId);
    }
}

record HealthResponse(String status) {}
