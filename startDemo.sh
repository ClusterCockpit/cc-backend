#!/bin/sh

rm -rf var

if [ -d './var' ]; then
  echo 'Directory ./var already exists! Skipping initialization.'
  ./cc-backend -server -dev
else
  make
  wget https://hpc-mover.rrze.uni-erlangen.de/HPC-Data/0x7b58aefb/eig7ahyo6fo2bais0ephuf2aitohv1ai/job-archive-dev.tar
  tar xf job-archive-dev.tar
  rm ./job-archive-dev.tar

  cp ./configs/env-template.txt .env
  cp ./configs/config-demo.json config.json

  echo 3 > /home/adityauj/cc-backend/var/job-archive/version.txt

  ./cc-backend --loglevel info -migrate-db
  ./cc-backend --loglevel info -dev -init-db -add-user demo:admin,api:demo
  
   # Generate JWT and extract only the token value
  JWT=$(./cc-backend -jwt demo | grep -oP "(?<=JWT: Successfully generated JWT for user 'demo': ).*")

  # Replace the existing JWT in test_ccms_write_api.sh with the new one
  if [ -n "$JWT" ]; then
    sed -i "1s|^JWT=.*|JWT=\"$JWT\"|" test_ccms_write_api.sh
    echo "✅ Updated JWT in test_ccms_write_api.sh"
  else
    echo "❌ Failed to generate JWT for demo user"
    exit 1
  fi

  ./cc-backend -server -dev

fi
