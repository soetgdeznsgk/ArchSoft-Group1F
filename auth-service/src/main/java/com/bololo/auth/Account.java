package com.bololo.auth;

import java.time.Instant;
import java.util.UUID;

record Account(UUID id, String username, String passwordHash, Instant lockedUntil) {
    Account(UUID id, String username, String passwordHash) {
        this(id, username, passwordHash, null);
    }
}
