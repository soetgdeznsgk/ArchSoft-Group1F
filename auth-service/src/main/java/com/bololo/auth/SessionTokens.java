package com.bololo.auth;

import java.security.interfaces.RSAPrivateKey;
import java.security.interfaces.RSAPublicKey;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;

import com.nimbusds.jose.jwk.JWKSet;
import com.nimbusds.jose.jwk.RSAKey;
import com.nimbusds.jose.jwk.source.ImmutableJWKSet;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.security.oauth2.jose.jws.SignatureAlgorithm;
import org.springframework.security.oauth2.jwt.JwsHeader;
import org.springframework.security.oauth2.jwt.JwtClaimsSet;
import org.springframework.security.oauth2.jwt.JwtEncoder;
import org.springframework.security.oauth2.jwt.JwtEncoderParameters;
import org.springframework.security.oauth2.jwt.NimbusJwtEncoder;
import org.springframework.stereotype.Component;

/**
 * Emite los tokens de sesión (decisiones 4 y 5). Auth es su único emisor: los firma con
 * RS256 y la clave privada de sesiones, que ningún otro servicio tiene.
 */
@Component
class SessionTokens {
    static final String SCOPE = "session";

    private final JwtEncoder encoder;
    private final Duration lifetime;
    private final Clock clock;

    @Autowired
    SessionTokens(
            @Value("${app.session-private-key-file}") String privateKeyFile,
            @Value("${app.session-public-key-file}") String publicKeyFile,
            @Value("${app.session-lifetime:PT12H}") Duration lifetime,
            Clock clock) {
        this(PemKeys.privateKey(privateKeyFile), PemKeys.publicKey(publicKeyFile), lifetime, clock);
    }

    SessionTokens(RSAPrivateKey privateKey, RSAPublicKey publicKey, Duration lifetime, Clock clock) {
        RSAKey key = new RSAKey.Builder(publicKey).privateKey(privateKey).build();
        this.encoder = new NimbusJwtEncoder(new ImmutableJWKSet<>(new JWKSet(key)));
        this.lifetime = lifetime;
        this.clock = clock;
    }

    /** `accountId` solo va en las sesiones con cuenta; las de invitado lo omiten. */
    String issue(String playerId, String nickname, String accountId) {
        Instant now = clock.instant();
        JwtClaimsSet.Builder claims = JwtClaimsSet.builder()
                .subject(playerId)
                .claim("scope", SCOPE)
                .claim("nickname", nickname)
                .issuedAt(now)
                .expiresAt(now.plus(lifetime));
        if (accountId != null) {
            claims.claim("accountId", accountId);
        }
        JwsHeader header = JwsHeader.with(SignatureAlgorithm.RS256).type("JWT").build();
        return encoder.encode(JwtEncoderParameters.from(header, claims.build())).getTokenValue();
    }
}
