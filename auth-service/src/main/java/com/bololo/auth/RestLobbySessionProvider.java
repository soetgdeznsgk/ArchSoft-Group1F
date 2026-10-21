package com.bololo.auth;

import java.util.UUID;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

@Component
class RestLobbySessionProvider implements LobbySessionProvider {
    private final RestClient client;

    RestLobbySessionProvider(
            RestClient.Builder builder,
            @Value("${app.lobby-manager-url}") String lobbyUrl,
            @Value("${app.service-secret}") String serviceSecret) {
        this.client = builder
                .baseUrl(lobbyUrl)
                .defaultHeader("X-Service-Token", serviceSecret)
                .build();
    }

    @Override
    public LobbySessionResponse createSession(UUID accountId, String nickname) {
        try {
            LobbySessionResponse response = client.post()
                    .uri("/internal/auth/session")
                    .body(new LobbySessionRequest(accountId.toString(), nickname))
                    .retrieve()
                    .body(LobbySessionResponse.class);
            if (response == null || response.player() == null || response.token() == null) {
                throw new AuthException(502, "lobby_unavailable", "The Lobby Manager returned an invalid session.");
            }
            return response;
        } catch (RestClientException failure) {
            throw new AuthException(502, "lobby_unavailable", "The Lobby Manager could not provide a player session.");
        }
    }
}
