package com.bololo.auth;

class AuthException extends RuntimeException {
    private final int status;
    private final String code;

    AuthException(int status, String code, String message) {
        super(message);
        this.status = status;
        this.code = code;
    }

    int status() {
        return status;
    }

    String code() {
        return code;
    }
}
