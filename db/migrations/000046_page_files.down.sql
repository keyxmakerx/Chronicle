-- Reverse 000046: drop the page file bindings. Files already attached stay in
-- media_files as unbound page files, which nothing serves and the orphan sweep
-- collects.
DROP TABLE IF EXISTS page_files;
