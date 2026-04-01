#!/bin/bash
set -e

cd /home/neo/Download/NogoChain

CHAIN_ID=3 \
GENESIS_PATH=genesis/smoke.json \
DATA_DIR=./data \
MINING_ENABLED=true \
MINER_ADDRESS=NOGO0049c3cf477a9fce2622d18245d04f011f788f7b2e248bdeb38d4ef459c37857be3d0293c3 \
P2P_ENABLE=true \
WS_ENABLE=true \
MAX_PEERS=50 \
RATE_LIMIT_REQUESTS=500 \
LOG_LEVEL=info \
./nogo
