-- Add is_public flag to campaigns so content can be viewed without login.
ALTER TABLE campaigns ADD COLUMN is_public BOOLEAN NOT NULL DEFAULT FALSE AFTER description;
