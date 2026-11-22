BEGIN;

DELETE FROM ratings WHERE account_id LIKE 'perf-%';

INSERT INTO ratings (account_id, mode, nickname, rating, games, wins)
SELECT
    'perf-' || lpad(i::text, 5, '0'),
    m.mode,
    'jugador_' || i,
    round((1500 + 300 * sqrt(-2 * ln(1 - random())) * cos(2 * pi() * random()))::numeric, 2),
    g.games,
    floor(random() * (g.games + 1))::int
FROM generate_series(1, 5000) AS i
CROSS JOIN (VALUES ('CLASSIC'), ('CHAMELEON')) AS m(mode)
CROSS JOIN LATERAL (SELECT 1 + floor(random() * 200)::int + 0 * i AS games) AS g;

COMMIT;

ANALYZE ratings;

SELECT mode, count(*) AS jugadores, round(min(rating)) AS min, round(avg(rating)) AS promedio, round(max(rating)) AS max
FROM ratings WHERE account_id LIKE 'perf-%' GROUP BY mode ORDER BY mode;
