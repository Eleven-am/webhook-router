-- OAuth2 service configurations for triggers
CREATE TABLE IF NOT EXISTS oauth2_services (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL, -- Will be encrypted by application
    token_url TEXT NOT NULL,
    auth_url TEXT,
    redirect_url TEXT,
    scopes TEXT, -- JSON array of scopes
    user_id TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, user_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Index for faster lookups
CREATE INDEX idx_oauth2_services_user_id ON oauth2_services(user_id);
CREATE INDEX idx_oauth2_services_name ON oauth2_services(name);