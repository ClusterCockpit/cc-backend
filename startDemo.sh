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

    ./cc-backend -migrate-db
    ./cc-backend -server -dev -init-db -add-user demo:admin:demo
fi
