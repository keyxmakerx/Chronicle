-- How the sidebar orders what sits directly inside a container: a category's
-- top level, a folder, or a page with pages under it. No row means A to Z.
-- 'hand' follows each row's sort_order; 'new' puts the newest first.
-- container_id is the category's type id as text, or the folder's or page's
-- id. It is deliberately not a foreign key: it names one of three tables, and
-- a row left behind by a deleted folder or page is never read again.
-- Core tables, core migration: only campaigns is referenced.

CREATE TABLE IF NOT EXISTS sidebar_child_orders (
    campaign_id    CHAR(36)    NOT NULL,
    container_kind ENUM('category','folder','page') NOT NULL,
    container_id   VARCHAR(36) NOT NULL,
    mode           ENUM('hand','new') NOT NULL,
    updated_at     DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (campaign_id, container_kind, container_id),
    CONSTRAINT fk_sidebar_child_orders_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
