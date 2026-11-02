UPDATE ai_connection_requests SET outcome = LEFT(outcome, 120) WHERE CHAR_LENGTH(outcome) > 120;
ALTER TABLE ai_connection_requests MODIFY COLUMN outcome VARCHAR(120) NOT NULL DEFAULT '';
