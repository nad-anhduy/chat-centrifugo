CREATE KEYSPACE IF NOT EXISTS chat_logs 
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE chat_logs;

CREATE TABLE IF NOT EXISTS messages (
    conversation_id uuid,
    created_at timestamp,
    message_id timeuuid,
    sender_id text,
    content_encrypted text,
    is_read boolean,
    PRIMARY KEY ((conversation_id), created_at, message_id)
) WITH CLUSTERING ORDER BY (created_at DESC);
