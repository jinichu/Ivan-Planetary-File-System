#!/bin/bash

source ./util.sh

./ssh.sh $1 "cd $PROJECT_PATH && ./ipfs localhost:8080"
