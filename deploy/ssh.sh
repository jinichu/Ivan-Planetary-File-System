#!/bin/bash

source ./util.sh

ip="$(ipn $1)"
sshvm $ip "${@:2}"
