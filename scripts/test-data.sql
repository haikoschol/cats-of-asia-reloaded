-- Fixtures for the isolated test database only, never normal app startup.
INSERT INTO photos_location (city, country, tzoffset)
VALUES
    ('Bali', 'Indonesia', -420),
    ('Vientiane', 'Laos', -420),
    ('Kuala Lumpur', 'Malaysia', -420)
ON CONFLICT (city, country) DO NOTHING;
