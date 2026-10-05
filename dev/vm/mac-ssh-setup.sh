#!/bin/bash
# client-mac (Tart, cirruslabs イメージ: admin/admin) に SSH 鍵を登録する。
set -euo pipefail
source "$(dirname "$0")/env.sh"
[ -f "$MAC_SSH_KEY" ] || ssh-keygen -q -t ed25519 -N "" -f "$MAC_SSH_KEY" -C secon-dev
ASKPASS=$STATE_DIR/askpass.sh
printf '#!/bin/sh\necho admin\n' >"$ASKPASS"
chmod +x "$ASKPASS"
SSH_ASKPASS=$ASKPASS SSH_ASKPASS_REQUIRE=force DISPLAY=:0 ssh -o StrictHostKeyChecking=accept-new \
  -o UserKnownHostsFile="$STATE_DIR/known_hosts" -o PubkeyAuthentication=no "admin@$(tart ip client-mac)" \
  "mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '$(cat "$MAC_SSH_KEY.pub")' >>~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys" </dev/null
mac_ssh sw_vers
