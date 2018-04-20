#!/bin/bash

source ./util.sh

for ip in $(cat ips.txt); do
	# Clean up the directory.
  sshvm $ip "rm -r $PROJECT_PATH/tmp/node1; echo foo"
done
