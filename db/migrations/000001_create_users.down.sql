-- Migration: 000001_create_users (rollback)
-- Description: Drops the users table. WARNING: destroys all user data.

DROP TABLE IF EXISTS users;
