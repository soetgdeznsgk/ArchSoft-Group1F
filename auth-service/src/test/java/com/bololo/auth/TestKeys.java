package com.bololo.auth;

import java.io.IOException;
import java.io.UncheckedIOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.NoSuchAlgorithmException;
import java.security.interfaces.RSAPrivateKey;
import java.security.interfaces.RSAPublicKey;
import java.util.Base64;

/** Pares RSA de prueba, escritos en PEM igual que scripts/gen-jwt-keys.sh. */
final class TestKeys {
    static final KeyPair SESSION = generate();
    static final KeyPair OTHER = generate();
    static final Path SESSION_PRIVATE_FILE = write("PRIVATE KEY", SESSION.getPrivate().getEncoded());
    static final Path SESSION_PUBLIC_FILE = write("PUBLIC KEY", SESSION.getPublic().getEncoded());

    private TestKeys() {}

    static RSAPrivateKey privateKey(KeyPair pair) {
        return (RSAPrivateKey) pair.getPrivate();
    }

    static RSAPublicKey publicKey(KeyPair pair) {
        return (RSAPublicKey) pair.getPublic();
    }

    private static KeyPair generate() {
        try {
            KeyPairGenerator generator = KeyPairGenerator.getInstance("RSA");
            generator.initialize(2048);
            return generator.generateKeyPair();
        } catch (NoSuchAlgorithmException impossible) {
            throw new IllegalStateException(impossible);
        }
    }

    private static Path write(String type, byte[] der) {
        String pem = "-----BEGIN " + type + "-----\n"
                + Base64.getMimeEncoder(64, "\n".getBytes(StandardCharsets.US_ASCII)).encodeToString(der)
                + "\n-----END " + type + "-----\n";
        try {
            Path file = Files.createTempFile("bololo-test-", ".pem");
            file.toFile().deleteOnExit();
            return Files.writeString(file, pem, StandardCharsets.US_ASCII);
        } catch (IOException failure) {
            throw new UncheckedIOException(failure);
        }
    }
}
