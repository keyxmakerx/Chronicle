-- 001_widget_bindings — the generic host ↔ widget-type ↔ data-instance table
-- (C-WIDGET-BINDING-P1-SPINE). Runs after core, so `campaign_id` references a
-- core table conceptually, but note:
--
-- FK-FREE BY DESIGN. host_id and instance_id are POLYMORPHIC: instance_id
-- points at calendars now, maps/timelines later; host_id at entities now,
-- entity_types/dashboards later. A hard FK is impossible across those tables
-- and would also trip the core-before-plugin migration-ordering rule. So there
-- are NO foreign keys here — referential integrity is enforced in the binding
-- service (per-plugin delete hook + render-time orphan guard + integrity
-- sweep). See .ai/decisions.md ADR 2026-06-07-widget-binding-polymorphic-fk-free.
--
-- host_type accepts 'entity' | 'entity_type' | 'dashboard' from day one (P1
-- only exercises 'entity'); widget_type is the registry slug ('calendar' in
-- P1). Both are validated in app code, NOT a DB enum, so new host/widget types
-- are added as data — no schema change (the modularity requirement).
CREATE TABLE IF NOT EXISTS widget_bindings (
    id           CHAR(36)    NOT NULL,
    campaign_id  CHAR(36)    NOT NULL,
    host_type    VARCHAR(32) NOT NULL,
    host_id      VARCHAR(64) NOT NULL,
    widget_type  VARCHAR(64) NOT NULL,
    instance_id  CHAR(36)    NOT NULL,
    created_at   TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    -- One binding per host per widget type (calendar is singleton-per-host).
    UNIQUE KEY uq_widget_binding_host (campaign_id, host_type, host_id, widget_type),
    -- Fast host resolution + delete-hook / sweep lookups.
    INDEX idx_widget_binding_host (campaign_id, host_type, host_id),
    INDEX idx_widget_binding_instance (campaign_id, widget_type, instance_id)
);
