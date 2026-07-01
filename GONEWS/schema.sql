CREATE TABLE IF NOT EXISTS posts (
    id SERIAL PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    content TEXT,
    pub_time TIMESTAMP NOT NULL,
    link VARCHAR(1000) UNIQUE NOT NULL,
    source VARCHAR(200),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_posts_pub_time ON posts(pub_time DESC);
CREATE INDEX IF NOT EXISTS idx_posts_link ON posts(link);