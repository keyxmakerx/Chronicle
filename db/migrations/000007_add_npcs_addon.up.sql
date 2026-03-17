-- Register the NPC Gallery as a toggleable addon so campaign owners can
-- enable/disable it from the Plugin Hub, just like calendar, maps, etc.
INSERT INTO addons (slug, name, description, version, category, status, icon, author)
VALUES ('npcs', 'NPC Gallery', 'Browse and reveal character entities as NPCs for your players.', '1.0.0', 'plugin', 'active', 'fa-users', 'Chronicle');
