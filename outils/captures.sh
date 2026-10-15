#!/bin/bash
# captures.sh <id> <tos> <machine> <disque A> <disque B|-> <auto|-> <touches|->
# Démarre le jeu sous Hatari 2.x (sans affichage), envoie des touches ST par la
# commande --cmd-fifo et enregistre des captures d'écran à intervalles.
# touches : "16:6 30:57" = scancode ST 6 (touche 5) à 16 s, Espace (57) à 30 s.
set -u
PACK="$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd)"
T="${TMPDIR:-/tmp}"
ID=$1; TOS=$2; MACH=$3; A=$4; B=${5:--}; AUTO=${6:--}; KEYS=${7:--}
[ "$B" = - ] && B=""; [ "$AUTO" = - ] && AUTO=""; [ "$KEYS" = - ] && KEYS=""
# amorce « Little Green Desktop » neutralisée, comme le fait le lanceur
if [ -z "${NOLGD:-}" ] && head -c 512 "$A" | grep -q "Little Green Desktop"; then
  cp "$A" "$T/lgd-$1.st"; python3 -c "import sys;f=open(sys.argv[1],'r+b');f.seek(511);b=f.read(1)[0];f.seek(511);f.write(bytes([b^1]))" "$T/lgd-$1.st"; A="$T/lgd-$1.st"
fi
OUT="$PACK/runtime/captures/$ID"; rm -rf "$OUT"; mkdir -p "$OUT"
F=$OUT/fifo
export SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy HOME="$T/hh-$ID"
mkdir -p "$HOME"
ARGS=(--configfile /dev/null --machine "$MACH" --tos "$TOS" --confirm-quit no --statusbar no --drive-led no
      --borders no --zoom 1 --sound off --fastfdc yes --fast-boot yes --screenshot-dir "$OUT" --cmd-fifo "$F" --disk-a "$A" ${EXTRA:-})
[ -n "$B" ] && ARGS+=(--disk-b "$B")
[ -n "$AUTO" ] && ARGS+=(--auto "$AUTO")
LD_LIBRARY_PATH="$PACK/emulateurs/hatari-linux" "$PACK/emulateurs/hatari-linux/hatari" "${ARGS[@]}" >"$OUT/log.txt" 2>&1 &
HP=$!
for i in $(seq 1 ${DUREE:-48}); do
  sleep 1; [ -p "$F" ] || continue
  for k in $KEYS; do
    if [ "${k%%:*}" = "$i" ]; then
      v=${k##*:}
      if [ "$v" = clic ]; then echo "hatari-event leftdown" > "$F"; sleep 0.3; echo "hatari-event leftup" > "$F"; else echo "hatari-event keydown $v" > "$F"; sleep 0.3; echo "hatari-event keyup $v" > "$F"; fi; sleep 0.3
    fi
  done
  if [ $(( i % 3 )) -eq 0 ] && [ $i -ge 6 ]; then echo "hatari-shortcut screenshot" > "$F"; fi
done
echo "hatari-shortcut quit" > "$F" 2>/dev/null; sleep 1; kill $HP 2>/dev/null; sleep 0.5; kill -9 $HP 2>/dev/null
rm -rf "$T/hh-$ID"
ls "$OUT" | grep -c png
