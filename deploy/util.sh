set -e
set -x

function ipn {
  sed "${1}q;d" ips.txt
}

function sshvm {
  sshpass -f password.txt ssh "$USER@$1" -C "$2"
}

PROJECT_PATH="~/proj2"
FIRST_MACHINE="$(ipn 1)"

USER=$(cat user.txt)
