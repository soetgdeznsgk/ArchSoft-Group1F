package com.bololo.auth;

class AuthException extends RuntimeException {
    private final int status;
    private final String code;
    private final Long retryAfterSeconds;

    AuthException(int status, String code, String message) {
        this(status, code, message, null);
    }

    AuthException(int status, String code, String message, Long retryAfterSeconds) {
        super(message);
        this.status = status;
        this.code = code;
        this.retryAfterSeconds = retryAfterSeconds;
    }

    int status() {
        return status;
    }

    String code() {
        return code;
    }

    Long retryAfterSeconds() {
        return retryAfterSeconds;
    }
}
