ALTER TABLE foundry_module_versions
    DROP KEY uk_github_release,
    DROP COLUMN github_release_id,
    DROP COLUMN github_release_tag,
    DROP COLUMN source,
    MODIFY COLUMN uploaded_by_user_id CHAR(36) NOT NULL;
