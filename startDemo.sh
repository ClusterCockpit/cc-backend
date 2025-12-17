#!/bin/sh

# rm -rf var

if [ -d './var' ]; then
  echo 'Directory ./var already exists! Skipping initialization.'
  ./cc-backend -server -dev -loglevel info
else
  make
  ./cc-backend --init
  cp ./configs/config-demo.json config.json

  wget https://hpc-mover.rrze.uni-erlangen.de/HPC-Data/0x7b58aefb/eig7ahyo6fo2bais0ephuf2aitohv1ai/job-archive-demo.tar
  tar xf job-archive-demo.tar
  rm ./job-archive-demo.tar

  ./cc-backend -dev -init-db -add-user demo:admin,api:demo
  ./cc-backend -server -dev -loglevel info
fi
