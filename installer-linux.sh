#!/bin/bash
# Ludothèque ST : démarrage automatique sous Linux.
#   bash installer-linux.sh             la ludothèque s'ouvre à l'ouverture de session
#   bash installer-linux.sh --kiosque   session « Ludothèque ST » dédiée, sans bureau,
#                                       ouverte automatiquement au démarrage (LightDM)
#   bash installer-linux.sh --retirer   enlève les deux
set -e
D="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"
chmod +x "$D/ludotheque.sh" "$D/ludotheque" "$D/emulateurs/hatari-linux/hatari" 2>/dev/null || true
AUTO="$HOME/.config/autostart/ludotheque-st.desktop"

case "${1:-}" in
--retirer)
  rm -f "$AUTO"
  if [ -f /usr/share/xsessions/ludotheque-st.desktop ] || [ -f /etc/lightdm/lightdm.conf.d/60-ludotheque-st.conf ]; then
    sudo rm -f /usr/share/xsessions/ludotheque-st.desktop /etc/lightdm/lightdm.conf.d/60-ludotheque-st.conf /usr/local/bin/ludotheque-st-session
  fi
  echo "Démarrage automatique retiré."
  ;;
--kiosque)
  echo "Session dédiée : il faut le mot de passe administrateur (sudo)."
  if ! command -v openbox >/dev/null; then
    echo "Installation d'Openbox (gestionnaire de fenêtres minimal)..."
    if command -v apt >/dev/null; then sudo apt install -y openbox; elif command -v pacman >/dev/null; then sudo pacman -S --noconfirm openbox; fi
  fi
  sudo tee /usr/local/bin/ludotheque-st-session >/dev/null <<EOS
#!/bin/bash
xset s off -dpms 2>/dev/null
openbox &
sleep 1
while true; do
  "$D/ludotheque" >/dev/null 2>&1
  [ -f "$D/runtime/quitter" ] && { rm -f "$D/runtime/quitter"; break; }
  sleep 1
done
EOS
  sudo chmod +x /usr/local/bin/ludotheque-st-session
  sudo tee /usr/share/xsessions/ludotheque-st.desktop >/dev/null <<EOS
[Desktop Entry]
Name=Ludothèque ST
Comment=Ludothèque Atari ST en plein écran
Exec=/usr/local/bin/ludotheque-st-session
Type=Application
EOS
  if [ -d /etc/lightdm ]; then
    sudo mkdir -p /etc/lightdm/lightdm.conf.d
    sudo tee /etc/lightdm/lightdm.conf.d/60-ludotheque-st.conf >/dev/null <<EOS
[Seat:*]
autologin-user=$USER
autologin-session=ludotheque-st
EOS
    getent group autologin >/dev/null && sudo usermod -aG autologin "$USER" || true
    echo "Au prochain démarrage, la ludothèque s'ouvre seule (session Ludothèque ST)."
  else
    echo "LightDM absent : choisissez la session « Ludothèque ST » sur l'écran de connexion."
  fi
  echo "Menu Fichier › Quitter la ludothèque : ferme la session (retour à l'écran de connexion)."
  ;;
*)
  mkdir -p "$(dirname "$AUTO")"
  cat > "$AUTO" <<EOS
[Desktop Entry]
Type=Application
Name=Ludothèque ST
Exec="$D/ludotheque.sh"
X-GNOME-Autostart-enabled=true
EOS
  echo "La ludothèque s'ouvrira à chaque ouverture de session."
  echo "Pour une console sans bureau : bash installer-linux.sh --kiosque"
  ;;
esac
