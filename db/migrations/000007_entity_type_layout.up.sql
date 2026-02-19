-- Migration: 000007_entity_type_layout
-- Description: Adds layout_json column to entity_types for customizable
--              entity profile page layouts (two-column editor with sections).

ALTER TABLE entity_types
    ADD COLUMN layout_json JSON NOT NULL DEFAULT ('{"sections":[]}') AFTER fields;
