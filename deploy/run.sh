#!/bin/bash

source ./util.sh

NUM=$1

CMD="cd $PROJECT_PATH && ./proj2 -bind :8080"

if [ "$NUM" -ne "1" ]; then
  CMD="$CMD -bootstrap $FIRST_MACHINE:8080"
fi

./ssh.sh $NUM "$CMD"
