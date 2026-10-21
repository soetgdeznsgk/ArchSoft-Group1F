package com.bololo.auth;

import java.util.UUID;

record Account(UUID id, String username, String passwordHash) {}
