#!/bin/sh

mkdir ./var
cd ./var

wget https://hpc-mover.rrze.uni-erlangen.de/HPC-Data/0x7b58aefb/eig7ahyo6fo2bais0ephuf2aitohv1ai/job-archive.tar.xz
tar xJf job-archive.tar.xz
rm ./job-archive.tar.xz

touch ./job.db
cd ../web/frontend
yarn install
yarn build

cd ../..
# Use your own keys in production!
cp ./configs/env-template.txt .env
go build ./cmd/cc-backend

./cc-backend --init-db --add-user demo:admin:AdminDev
