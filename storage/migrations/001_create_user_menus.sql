-- Migration: Create user_menus table
-- This table stores user menu data as JSONB

CREATE TABLE IF NOT EXISTS user_menus (
    user_id BIGINT PRIMARY KEY,
    menu_data JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create index on user_id for faster lookups (already primary key, but explicit for clarity)
CREATE INDEX IF NOT EXISTS idx_user_menus_user_id ON user_menus(user_id);

-- Create GIN index on menu_data for efficient JSONB queries (optional, useful for future queries)
CREATE INDEX IF NOT EXISTS idx_user_menus_menu_data ON user_menus USING GIN (menu_data);

