INSERT INTO users (id, name, email) VALUES (1, 'Duplicate', 'duplicate@example.com') ON CONFLICT (id) DO NOTHING;
