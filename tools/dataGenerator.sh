#!/bin/bash

# ==========================================
# CONFIGURATION & FLAGS
# ==========================================

# MODE SETTINGS
TRANSPORT_MODE="REST"       # Options: "REST" or "NATS"
CONNECTION_SCOPE="INTERNAL" # Options: "INTERNAL" or "EXTERNAL"
API_USER="demo"             # User for JWT generation

# BASE NETWORK CONFIG
SERVICE_ADDRESS="http://localhost:8080"
NATS_SERVER="nats://0.0.0.0:4222"
REST_URL="${SERVICE_ADDRESS}/api/write"

# NATS CREDENTIALS
NATS_USER="root"
NATS_PASS="root"
NATS_SUBJECT="hpc-nats"

# EXTERNAL JWT (Used if CONNECTION_SCOPE is EXTERNAL)
JWT_STATIC="eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NzU3Nzg4NDQsImlhdCI6MTc2ODU3ODg0NCwicm9sZXMiOlsiYWRtaW4iLCJhcGkiXSwic3ViIjoiZGVtbyJ9._SDEW9WaUVXSBFmWqGhyIZXLoqoDU8F1hkfh4cXKIqF4yw7w50IUpfUBtwUFUOnoviFKoi563f6RAMC7XxeLDA"

# ==========================================
# DATA DEFINITIONS
# ==========================================
ALEX_HOSTS="a0603 a0903 a0832 a0329 a0702 a0122 a1624 a0731 a0224 a0704 a0631 a0225 a0222 a0427 a0603 a0429 a0833 a0705 a0901 a0601 a0227 a0804 a0322 a0226 a0126 a0129 a0605 a0801 a0934 a1622 a0902 a0428 a0537 a1623 a1722 a0228 a0701 a0326 a0327 a0123 a0321 a1621 a0323 a0124 a0534 a0931 a0324 a0933 a0424 a0905 a0128 a0532 a0805 a0521 a0535 a0932 a0127 a0325 a0633 a0831 a0803 a0426 a0425 a0229 a1721 a0602 a0632 a0223 a0422 a0423 a0536 a0328 a0703 anvme7 a0125 a0221 a0604 a0802 a0522 a0531 a0533 a0904"
FRITZ_HOSTS="f0201 f0202 f0203 f0204 f0205 f0206 f0207 f0208 f0209 f0210 f0211 f0212 f0213 f0214 f0215 f0217 f0218 f0219 f0220 f0221 f0222 f0223 f0224 f0225 f0226 f0227 f0228 f0229 f0230 f0231 f0232 f0233 f0234 f0235 f0236 f0237 f0238 f0239 f0240 f0241 f0242 f0243 f0244 f0245 f0246 f0247 f0248 f0249 f0250 f0251 f0252 f0253 f0254 f0255 f0256 f0257 f0258 f0259 f0260 f0261 f0262 f0263 f0264 f0378"

ALEX_METRICS_HWTHREAD="cpu_user flops_any clock core_power ipc"
ALEX_METRICS_SOCKET="mem_bw cpu_power"
ALEX_METRICS_ACC="acc_utilization acc_mem_used acc_power nv_mem_util nv_temp nv_sm_clock"
ALEX_METRICS_NODE="cpu_load mem_used net_bytes_in net_bytes_out"

FRITZ_METRICS_HWTHREAD="cpu_user flops_any flops_sp flops_dp clock ipc vectorization_ratio"
FRITZ_METRICS_SOCKET="mem_bw cpu_power mem_power"
FRITZ_METRICS_NODE="cpu_load mem_used ib_recv ib_xmit ib_recv_pkts ib_xmit_pkts nfs4_read nfs4_total"

ACCEL_IDS="00000000:49:00.0 00000000:0E:00.0 00000000:D1:00.0 00000000:90:00.0 00000000:13:00.0 00000000:96:00.0 00000000:CC:00.0 00000000:4F:00.0"

# ==========================================
# SETUP ENV (URL & TOKEN)
# ==========================================

if [ "$CONNECTION_SCOPE" == "INTERNAL" ]; then    
    # 2. Generate JWT dynamically
    echo "Setup: INTERNAL mode selected."
    echo "Generating JWT for user: $API_USER"
    JWT=$(./cc-backend -jwt "$API_USER" | grep -oP "(?<=JWT: Successfully generated JWT for user '${API_USER}': ).*")
    
    if [ -z "$JWT" ]; then
        echo "Error: Failed to generate JWT from cc-backend."
        exit 1
    fi
else    
    # 2. Use Static JWT
    echo "Setup: EXTERNAL mode selected."
    echo "Using static JWT."
    JWT="$JWT_STATIC"
fi

echo "Target URL: $REST_URL"

# ==========================================
# FUNCTIONS
# ==========================================

send_payload() {
    local file_path=$1
    local cluster_name=$2

    if [ "$TRANSPORT_MODE" == "NATS" ]; then
        # Piping file content directly to nats stdin
        cat "$file_path" | nats pub "$NATS_SUBJECT" -s "$NATS_SERVER" --user "$NATS_USER" --password "$NATS_PASS"
    else
        # Sending via REST API
        curl -s -X 'POST' "${REST_URL}/?cluster=${cluster_name}" \
            -H "Authorization: Bearer $JWT" \
            --data-binary "@$file_path"
    fi
    
    # Clean up immediately
    rm "$file_path"
}

# ==========================================
# MAIN LOOP
# ==========================================

# Clean up leftovers
rm -f sample_fritz.txt sample_alex.txt

while [ true ]; do
    timestamp="$(date '+%s')"
    echo "--- Cycle Start: $timestamp [Mode: $TRANSPORT_MODE | Scope: $CONNECTION_SCOPE] ---"

    # 1. ALEX: HWTHREAD
    echo "Generating Alex: hwthread"
    {
        for metric in $ALEX_METRICS_HWTHREAD; do
            for hostname in $ALEX_HOSTS; do
                for id in {0..127}; do
                    echo "$metric,cluster=alex,hostname=$hostname,type=hwthread,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp"
                done
            done
        done
    } > sample_alex.txt
    send_payload "sample_alex.txt" "alex"

    # 2. FRITZ: HWTHREAD
    echo "Generating Fritz: hwthread"
    {
        for metric in $FRITZ_METRICS_HWTHREAD; do
            for hostname in $FRITZ_HOSTS; do
                for id in {0..71}; do
                    echo "$metric,cluster=fritz,hostname=$hostname,type=hwthread,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp"
                done
            done
        done
    } > sample_fritz.txt
    send_payload "sample_fritz.txt" "fritz"

    # 3. ALEX: ACCELERATOR
    echo "Generating Alex: accelerator"
    {
        for metric in $ALEX_METRICS_ACC; do
            for hostname in $ALEX_HOSTS; do
                for id in $ACCEL_IDS; do
                    echo "$metric,cluster=alex,hostname=$hostname,type=accelerator,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp"
                done
            done
        done
    } > sample_alex.txt
    send_payload "sample_alex.txt" "alex"

    # 5. ALEX: SOCKET
    echo "Generating Alex: socket"
    {
        for metric in $ALEX_METRICS_SOCKET; do
            for hostname in $ALEX_HOSTS; do
                for id in {0..1}; do
                    echo "$metric,cluster=alex,hostname=$hostname,type=socket,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp"
                done
            done
        done
    } > sample_alex.txt
    send_payload "sample_alex.txt" "alex"

    # 6. FRITZ: SOCKET
    echo "Generating Fritz: socket"
    {
        for metric in $FRITZ_METRICS_SOCKET; do
            for hostname in $FRITZ_HOSTS; do
                for id in {0..1}; do
                    echo "$metric,cluster=fritz,hostname=$hostname,type=socket,type-id=$id value=$((1 + RANDOM % 100)).0 $timestamp"
                done
            done
        done
    } > sample_fritz.txt
    send_payload "sample_fritz.txt" "fritz"

    # 7. ALEX: NODE
    echo "Generating Alex: node"
    {
        for metric in $ALEX_METRICS_NODE; do
            for hostname in $ALEX_HOSTS; do
                echo "$metric,cluster=alex,hostname=$hostname,type=node value=$((1 + RANDOM % 100)).0 $timestamp"
            done
        done
    } > sample_alex.txt
    send_payload "sample_alex.txt" "alex"

    # 8. FRITZ: NODE
    echo "Generating Fritz: node"
    {
        for metric in $FRITZ_METRICS_NODE; do
            for hostname in $FRITZ_HOSTS; do
                echo "$metric,cluster=fritz,hostname=$hostname,type=node value=$((1 + RANDOM % 100)).0 $timestamp"
            done
        done
    } > sample_fritz.txt
    send_payload "sample_fritz.txt" "fritz"

    sleep 1m
done