-- Reverse 000068. Dropping entity_set_aside_values loses the values a switch
-- took off pages; a category still switched keeps its new fields. Switch the
-- categories back first to put those values on their pages again.
DROP TABLE IF EXISTS entity_set_aside_values;
DROP TABLE IF EXISTS entity_type_field_switches;
