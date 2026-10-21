package com.bololo.auth;

import java.util.Optional;
import java.util.UUID;

import org.springframework.dao.EmptyResultDataAccessException;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Repository;

@Repository
class AccountRepository {
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
        try {
            Account account = jdbc.queryForObject(
                    "SELECT id, username, password_hash FROM accounts WHERE username = ?",
                    (result, rowNumber) -> new Account(
                            result.getObject("id", UUID.class),
                            result.getString("username"),
                            result.getString("password_hash")),
                    username);
            return Optional.ofNullable(account);
        } catch (EmptyResultDataAccessException notFound) {
            return Optional.empty();
        }
    }
}
