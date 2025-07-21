-- Fulcrum Framework Database Initialization
-- This script runs when the PostgreSQL container starts for the first time

-- Create additional databases for testing
CREATE DATABASE fulcrum_test;

-- Grant permissions
GRANT ALL PRIVILEGES ON DATABASE fulcrum_dev TO fulcrum;
GRANT ALL PRIVILEGES ON DATABASE fulcrum_test TO fulcrum;

-- Extensions we might need
-- CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
-- CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Log initialization
SELECT 'Fulcrum database initialized successfully!' as message;
