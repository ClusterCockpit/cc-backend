#!/bin/sh

mkdir ./var
cd ./var

wget https://hpc-mover.rrze.uni-erlangen.de/HPC-Data/0x7b58aefb/eig7ahyo6fo2bais0ephuf2aitohv1ai/job-archive-dev.tar.xz
tar xJf job-archive.tar.xz
rm ./job-archive.tar.xz

touch ./job.db
cd ../web/frontend
yarn install
yarn build

cd ../..
cp ./configs/env-template.txt .env
cp ./docs/config.json config.json
go build ./cmd/cc-backend

./cc-backend --server --dev --init-db --add-user demo:admin:AdminDev