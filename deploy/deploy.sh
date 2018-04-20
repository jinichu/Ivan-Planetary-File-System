#!/bin/bash

source ./util.sh

make build -C ..

for ip in $(cat ips.txt); do
  # setup go
  sshvm $ip "curl -sL -o ~/gimme https://raw.githubusercontent.com/travis-ci/gimme/master/gimme &&
    chmod +x gimme &&
    ~/gimme 1.10.1 &&
    mkdir -p ~/go"

  # transfer code
  sshpass -f password.txt rsync -ravz .. $USER@$ip:$PROJECT_PATH --exclude .git --exclude tmp
done
