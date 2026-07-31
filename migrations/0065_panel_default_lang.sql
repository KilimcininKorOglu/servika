-- Server-default panel language. Before a user has authenticated (e.g. the login
-- screen) the frontend reads this to decide which language to open in. It is asked
-- during installation and an admin can change it later. A signed-in user's own
-- pref_lang (users.pref_lang) always overrides this; this is only the shared
-- first-impression default. English is the panel's primary language.
ALTER TABLE panel_settings ADD COLUMN default_lang VARCHAR(8) NOT NULL DEFAULT 'en';
