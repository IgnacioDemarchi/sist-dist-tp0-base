#!/usr/bin/env bash
set -euo pipefail
OUT="${1:-docker-compose-dev.yaml}"
N="${2:-5}"

cat > "$OUT" <<'YAML'
name: tp0
services:
  server:
    container_name: server
    image: server:latest
    entrypoint: python3 /main.py
    environment:
      - PYTHONUNBUFFERED=1
      - LOGGING_LEVEL=INFO
    volumes:
      - ./server/config.ini:/config.ini:ro
    networks: [testing_net]
YAML

for i in $(seq 1 "$N"); do
cat >> "$OUT" <<YAML
  client$i:
    container_name: client$i
    image: client:latest
    entrypoint: /client
    environment:
      - CLI_ID=$i
      - CLI_LOG_LEVEL=INFO
    volumes:
      - ./client/config.yaml:/config.yaml:ro
    networks: [testing_net]
    depends_on: [server]
YAML
done

cat >> "$OUT" <<'YAML'
networks:
  testing_net:
    ipam:
      driver: default
      config:
        - subnet: 172.25.125.0/24
YAML

echo "Generated $OUT with $N clients."