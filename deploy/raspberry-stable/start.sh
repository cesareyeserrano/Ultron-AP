#!/bin/bash
pkill -f ultron-ap 2>/dev/null || true
sleep 1
export ULTRON_DB_PATH="$HOME/ultron-ap/data/ultron.db"
export ULTRON_ADMIN_PASS="admin123"
export ULTRON_PORT="8080"
nohup "$HOME/ultron-ap/ultron-ap" >> "$HOME/ultron-ap/ultron.log" 2>&1 &
echo $! > "$HOME/ultron-ap/ultron.pid"
echo "Started PID $(cat $HOME/ultron-ap/ultron.pid)"
