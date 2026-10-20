-- Member look picks: what one person chose for themselves in one campaign,
-- per part of the look (the character sheet style, the DM Screen's style
-- kit). A pick changes only what that person sees, so it is keyed by
-- campaign and user together; whether it is honoured is decided at render
-- time from the campaign's "who can change" setting, which is why a narrowed
-- setting never needs this table cleaned. value holds the part's own choice
-- id; the service validates it against that part's allowed list.
-- Core tables, core migration: no plugin tables referenced.

CREATE TABLE IF NOT EXISTS campaign_member_look_picks (
    campaign_id CHAR(36)    NOT NULL,
    user_id     CHAR(36)    NOT NULL,
    part        VARCHAR(32) NOT NULL,
    value       VARCHAR(64) NOT NULL,
    updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id, user_id, part),
    KEY idx_member_look_picks_user (user_id),
    CONSTRAINT fk_member_look_picks_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_member_look_picks_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
