#!/bin/bash
# Ludothèque ST : lancement sous Linux (Mint, Manjaro...).
D="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"
chmod +x "$D/ludotheque" "$D/emulateurs/hatari-linux/hatari" "$D/reforged/stuntcar-natif/scr" 2>/dev/null
exec "$D/ludotheque" "$@"
