#!/bin/bash

echo "=== INITIALIZING INFRASTRUCTURE ==="

# 1. Wait for ScyllaDB to be ready
echo "[1/3] Waiting for ScyllaDB to accept connections..."
MAX_RETRY=30
RETRY_COUNT=0
while ! docker-compose exec -T scylladb cqlsh -e 'describe cluster' > /dev/null 2>&1; do
    RETRY_COUNT=$((RETRY_COUNT+1))
    if [ $RETRY_COUNT -ge $MAX_RETRY ]; then
        echo "Error: ScyllaDB failed to start within time limit."
        exit 1
    fi
    echo "ScyllaDB not ready yet... retrying in 3s ($RETRY_COUNT/$MAX_RETRY)"
    sleep 3
done
echo "ScyllaDB is ready!"

# 2. Run CQL Init script
echo "[2/3] Initializing ScyllaDB keyspace and tables..."
# Using the mounted script inside the container to create schema
docker-compose exec -T scylladb cqlsh -f /docker-entrypoint-initdb.d/init-scylla.cql
echo "ScyllaDB initialization complete."

# 3. Check Redis connection
echo "[3/3] Checking Redis and Centrifugo..."
if docker-compose exec -T redis redis-cli ping > /dev/null 2>&1; then
    echo "Redis is UP and responding."
else
    echo "Error: Redis is DOWN or unreachable."
fi

# Check Centrifugo status (using docker-compose ps since curl might not be available inside containers)
if docker-compose ps centrifugo | grep -q 'Up'; then
    echo "Centrifugo container is UP."
else
    echo "Error: Centrifugo container is DOWN."
fi

echo "=== INFRASTRUCTURE INITIALIZATION COMPLETE ==="
