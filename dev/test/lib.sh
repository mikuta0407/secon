# テストスクリプト共通。macOS には timeout(1) が無いので代替を用意する。
if ! command -v timeout >/dev/null 2>&1; then
  # timeout SECS CMD...: SECS 後に SIGTERM を送る (終了処理が走るように KILL ではなく TERM)
  timeout() {
    local secs=$1; shift
    "$@" &
    local pid=$!
    ( sleep "$secs"; kill -TERM "$pid" 2>/dev/null ) &
    local wd=$!
    wait "$pid"; local rc=$?
    kill "$wd" 2>/dev/null
    return $rc
  }
fi
