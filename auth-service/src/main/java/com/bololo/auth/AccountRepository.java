package com.bololo.auth;

import java.sql.ResultSet;
import java.sql.SQLException;
import java.time.Instant;
import java.time.OffsetDateTime;
import java.time.ZoneOffset;
import java.util.Optional;
import java.util.UUID;

import org.springframework.dao.EmptyResultDataAccessException;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Repository;

@Repository
class AccountRepository {
    private static final String SELECT_ACCOUNT =
            "SELECT id, username, password_hash, locked_until FROM accounts WHERE ";

    private final JdbcTemplate jdbc;

    AccountRepository(JdbcTemplate jdbc) {
        this.jdbc = jdbc;
    }

    void create(Account account) {
        jdbc.update(
                "INSERT INTO accounts (id, username, password_hash) VALUES (?, ?, ?)",
                account.id(),
                account.username(),
                account.passwordHash());
    }

    Optional<Account> findByUsername(String username) {
        return findOne("username = ?", username);
    }

    Optional<Account> findById(UUID id) {
        return findOne("id = ?", id);
    }

    void recordFailedLogin(UUID id, int maxAttempts, Instant lockUntil) {
        jdbc.update(
                """
                UPDATE accounts SET
                    locked_until = CASE WHEN failed_logins + 1 >= ? THEN ? ELSE locked_until END,
                    failed_logins = CASE WHEN failed_logins + 1 >= ? THEN 0 ELSE failed_logins + 1 END
                WHERE id = ?
                """,
                maxAttempts,
                OffsetDateTime.ofInstant(lockUntil, ZoneOffset.UTC),
                maxAttempts,
                id);
    }

    void clearFailedLogins(UUID id) {
        jdbc.update("UPDATE accounts SET failed_logins = 0, locked_until = NULL WHERE id = ?", id);
    }

    void updatePassword(UUID id, String passwordHash) {
        jdbc.update("UPDATE accounts SET password_hash = ? WHERE id = ?", passwordHash, id);
    }

    private Optional<Account> findOne(String condition, Object value) {
        try {
            return Optional.ofNullable(jdbc.queryForObject(SELECT_ACCOUNT + condition, AccountRepository::toAccount, value));
        } catch (EmptyResultDataAccessException notFound) {
            return Optional.empty();
        }
    }

    private static Account toAccount(ResultSet result, int rowNumber) throws SQLException {
        OffsetDateTime lockedUntil = result.getObject("locked_until", OffsetDateTime.class);
        return new Account(
                result.getObject("id", UUID.class),
                result.getString("username"),
                result.getString("password_hash"),
                lockedUntil == null ? null : lockedUntil.toInstant());
    }
}
