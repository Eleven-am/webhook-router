-- Add grant_type column to oauth2_services table for PostgreSQL
ALTER TABLE oauth2_services ADD COLUMN grant_type TEXT NOT NULL DEFAULT 'client_credentials';