#!/bin/sh

if [ -d './var' ]; then
    echo 'Directory ./var already exists! Skipping initialization.'
    ./cc-backend -server -dev
else
    make
    wget https://hpc-mover.rrze.uni-erlangen.de/HPC-Data/0x7b58aefb/eig7ahyo6fo2bais0ephuf2aitohv1ai/job-archive-demo.tar
    tar xf job-archive-demo.tar
    rm ./job-archive-demo.tar

    cp ./configs/env-template.txt .env
    cp ./configs/config-demo.json config.json

    # mkdir -p ./var/checkpoints
    # cp -rf ~/cc-metric-store/var/checkpoints ~/cc-backend/var

    ./cc-backend -migrate-db
    ./cc-backend -dev -init-db -add-user demo:admin,api:demo

  # --- begin: generate JWT for demo and update test_ccms_write_api.sh ---
    CC_BIN="./cc-backend"
    TEST_FILE="./test_ccms_write_api.sh"
    BACKUP_FILE="${TEST_FILE}.bak"

    if [ -x "$CC_BIN" ]; then
        echo "Generating JWT for user 'demo'..."
        output="$($CC_BIN -jwt demo 2>&1 || true)"
        token="$(printf '%s\n' "$output" | grep -oE '[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+' | head -n1 || true)"

        if [ -z "$token" ]; then
            echo "Warning: could not extract JWT from output:" >&2
            printf '%s\n' "$output" >&2
        else
            if [ -f "$TEST_FILE" ]; then
                cp -a "$TEST_FILE" "$BACKUP_FILE"
                # replace first line with JWT="..."
                sed -i "1s#.*#JWT=\"$token\"#" "$TEST_FILE"
                echo "Updated JWT in $TEST_FILE (backup at $BACKUP_FILE)"
            else
                echo "Warning: $TEST_FILE not found; JWT not written."
            fi
        fi
    else
        echo "Warning: $CC_BIN not found or not executable; skipping JWT generation."
    fi
    # --- end: generate JWT for demo and update test_ccms_write_api.sh ---


    ./cc-backend -server -dev

fi
