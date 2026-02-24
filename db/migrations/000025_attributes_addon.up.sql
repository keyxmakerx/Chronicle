-- Register the "attributes" addon so campaign owners can toggle it via Extensions.
-- When disabled, attribute fields are hidden from entity pages in that campaign.
INSERT INTO addons (slug, name, description, version, category, status, icon, author)
VALUES ('attributes', 'Attributes', 'Custom attribute fields on entity pages (e.g. race, alignment, HP). When disabled, attribute panels are hidden from entity pages.', '0.1.0', 'widget', 'active', 'fa-sliders', 'Chronicle');
