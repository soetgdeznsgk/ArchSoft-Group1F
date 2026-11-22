BEGIN;
DELETE FROM rating_changes WHERE game_id LIKE 'perf-%' OR account_id LIKE 'perf-%';
DELETE FROM processed_games WHERE game_id LIKE 'perf-%';
DELETE FROM ratings WHERE account_id LIKE 'perf-%';
COMMIT;
