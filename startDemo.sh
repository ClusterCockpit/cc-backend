#!/bin/sh

mkdir ./var
cd ./var

wget https://hpc-mover.rrze.uni-erlangen.de/HPC-Data/0x7b58aefb/eig7ahyo6fo2bais0ephuf2aitohv1ai/job-archive.tar.xz
tar xJf job-archive.tar.xz
rm ./job-archive.tar.xz

touch ./job.db
cd ../frontend
yarn install
yarn build

cd ..
go get
go build

./cc-backend --init-db --add-user demo:admin:AdminDev --no-server
./cc-backend
