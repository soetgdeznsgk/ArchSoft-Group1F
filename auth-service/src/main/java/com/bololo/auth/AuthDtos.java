package com.bololo.auth;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Pattern;
import jakarta.validation.constraints.Size;

record CredentialsRequest(
        @NotBlank
        @Size(min = 3, max = 20)
        @Pattern(regexp = "^[A-Za-z0-9_.-]+$")
        String username,
        @NotBlank String password) {}

record RegisterRequest(
        @NotBlank
        @Size(min = 3, max = 20)
        @Pattern(regexp = "^[A-Za-z0-9_.-]+$")
        String username,
        @NotBlank String password) {}

record ChangePasswordRequest(@NotBlank String currentPassword, @NotBlank String newPassword) {}

record PlayerResponse(String id, String nickname) {}

record AccountResponse(String id, String username) {}

record SessionResponse(PlayerResponse player, String token, AccountResponse account) {}

record ErrorResponse(String code, String message) {}

record LobbySessionRequest(String accountId, String nickname) {}

record LobbySessionResponse(PlayerResponse player, String token) {}
