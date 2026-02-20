-- Mark the player-notes addon as active now that the widget is implemented.
UPDATE addons SET status = 'active' WHERE slug = 'player-notes';
