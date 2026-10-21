package com.bololo.auth;

import jakarta.validation.Valid;

import org.springframework.http.HttpStatus;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
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
    @org.springframework.web.bind.annotation.ResponseStatus(HttpStatus.CREATED)
    SessionResponse register(@Valid @RequestBody RegisterRequest request) {
        return authService.register(request.username(), request.password());
    }

    @PostMapping("/auth/login")
    SessionResponse login(@Valid @RequestBody CredentialsRequest request) {
        return authService.login(request.username(), request.password());
    }

    @GetMapping("/healthz")
    HealthResponse health() {
        jdbc.queryForObject("SELECT 1", Integer.class);
        return new HealthResponse("ok");
    }
}

record HealthResponse(String status) {}
