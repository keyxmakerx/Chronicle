-- Add prerelease flag to package_versions so pre-release/beta versions
-- are tracked and displayed distinctly from stable releases.
ALTER TABLE package_versions
    ADD COLUMN prerelease BOOLEAN NOT NULL DEFAULT FALSE AFTER release_notes;
