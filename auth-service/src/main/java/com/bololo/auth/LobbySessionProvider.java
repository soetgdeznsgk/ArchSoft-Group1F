package com.bololo.auth;

import java.util.UUID;

interface LobbySessionProvider {
    LobbySessionResponse createSession(UUID accountId, String nickname);
}
