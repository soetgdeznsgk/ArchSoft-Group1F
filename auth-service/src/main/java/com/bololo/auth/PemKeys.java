package com.bololo.auth;

import java.io.IOException;
import java.io.UncheckedIOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.GeneralSecurityException;
import java.security.KeyFactory;
import java.security.interfaces.RSAPrivateKey;
import java.security.interfaces.RSAPublicKey;
import java.security.spec.PKCS8EncodedKeySpec;
import java.security.spec.X509EncodedKeySpec;
import java.util.Base64;

/** Lee las claves RSA en PEM que genera scripts/gen-jwt-keys.sh (PKCS#8 y SubjectPublicKeyInfo). */
final class PemKeys {
    private PemKeys() {}

    static RSAPrivateKey privateKey(String location) {
        try {
            byte[] der = der(location, "PRIVATE KEY");
            return (RSAPrivateKey) KeyFactory.getInstance("RSA").generatePrivate(new PKCS8EncodedKeySpec(der));
        } catch (GeneralSecurityException | ClassCastException invalid) {
            throw new IllegalStateException("La clave privada RSA de " + location + " no es válida", invalid);
        }
    }

    static RSAPublicKey publicKey(String location) {
        try {
            byte[] der = der(location, "PUBLIC KEY");
            return (RSAPublicKey) KeyFactory.getInstance("RSA").generatePublic(new X509EncodedKeySpec(der));
        } catch (GeneralSecurityException | ClassCastException invalid) {
            throw new IllegalStateException("La clave pública RSA de " + location + " no es válida", invalid);
        }
    }

    private static byte[] der(String location, String type) {
        String pem;
        try {
            pem = Files.readString(Path.of(location), StandardCharsets.US_ASCII);
        } catch (IOException missing) {
            throw new UncheckedIOException("No se pudo leer la clave " + location, missing);
        }
        String begin = "-----BEGIN " + type + "-----";
        String end = "-----END " + type + "-----";
        int from = pem.indexOf(begin);
        int to = pem.indexOf(end);
        if (from < 0 || to < from) {
            throw new IllegalStateException(location + " no contiene un bloque PEM " + type);
        }
        return Base64.getMimeDecoder().decode(pem.substring(from + begin.length(), to));
    }
}
