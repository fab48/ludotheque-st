# Ludothèque ST — notes pour Claude Code

Surcouche « bureau GEM » pour l'émulateur Atari ST **Hatari** : un bac de disquettes animé
dans le navigateur (plein écran, mode kiosque), une fiche par jeu, et Hatari (ou une version
« reforged » web/native) lancé en plein écran. Pack **100 % portable** : Windows et Linux
(Mint, Manjaro), rien à installer. Projet de Fab ; tout est en **français** (interface,
journaux, commentaires, noms de variables Go/JS).

## D'où vient le travail fait jusqu'ici

Tout a été développé le 12/10/2026 dans une conversation de l'app Claude (claude.ai), pas
dans Claude Code : Claude y travaillait dans un espace de travail Linux dans le cloud (Go
1.24.7, mingw-w64, chroot Ubuntu 22.04 pour Hatari Linux, Xvfb, Playwright) et déposait les
fichiers dans ce dossier via le lien « ordinateur » de l'app de bureau. D'où les binaires
Windows compilés depuis Linux. Cet espace cloud n'est pas accessible depuis Claude Code et
disparaît avec la conversation : **les sources de ce dossier font foi**.

Sur ce PC Windows : installer Go (`winget install GoLang.Go`) suffit pour le lanceur
(`build.bat`). Recompiler Hatari demande MSYS2/mingw-w64 + CMake + SDL2 (Windows) ou une
machine Ubuntu 22.04 (Linux) ; ne le faire que si on touche aux patchs.

## Règles de travail

- Répondre en français. Textes d'interface courts, ton « Atari ST ».
- **Ne jamais effacer** de fichiers du pack : déplacer ce qui est à jeter dans
  `runtime/_a_supprimer/`.
- Ne pas committer/pousser sans qu'on le demande.
- `Ludotheque.exe` est verrouillé tant que la ludothèque tourne : la fermer
  (Fichier › Quitter, ou Échap maintenu hors jeu) avant de le remplacer. Sinon le renommer
  dans `runtime/_a_supprimer/` (Windows autorise le renommage d'un exe en cours) puis écrire
  le nouveau.
- Après une modif du lanceur, **le relancer** : l'interface appelle des routes qui
  n'existent pas dans un vieux lanceur (symptôme : « erreur 404 »).
- Tester ce qui peut l'être avant de livrer (captures Hatari, Playwright pour l'UI).

## Arborescence

```
Ludotheque.exe / ludotheque     lanceur (Windows / Linux), un seul binaire Go chacun
ludotheque.sh, installer-linux.sh, "Demarrage automatique - *.bat"
config.json                     réglages (voir plus bas)
catalogue.json                  jeux, versions (variantes), cases cochées (F10) — GÉNÉRÉ
demos.json, demos/              35 démos célèbres (TOSEC) : veille et Play Demo Random (F7)
ui/index.html                   TOUTE l'interface (HTML/CSS/JS vanilla, une page, scène 1280×720)
ui/polices, ui/sons             VT323/Caveat, disquette.wav (son du dépôt shufflepuck)
medias/<id>/                    captures des fiches (1.png, 2.png fournies ; grab*.png = F8 en jeu)
disques/                        images .ST/.MSA (beaucoup de compils Automation « A_xxx_*.ST »)
disques/tosec/                  ~150 images choisies dans la collection TOSEC 2012 (13/10/2026)
tos/                            tos162fr.img (défaut, force STE), tos104fr/tos102fr (STF, TOSEC),
                                tos206fr.img, tos400/404, emutos
emulateurs/hatari-windows       Hatari 2.6.1 PATCHÉ (hatari.exe + SDL2.dll)
emulateurs/hatari-linux         Hatari 2.6.1 PATCHÉ (glibc ≤ 2.35, + libSDL2 livrée)
emulateurs/hatari-1.8-windows   ancien Hatari, repli
reforged/                       shufflepuck (HTML unique), stuntcar-web, stuntcar-natif, ikplus
runtime/                        journal ludotheque.log, profil navigateur, cfg Hatari générée,
                                runtime/disques/ (copies nettoyées/patchées des disquettes —
                                LES SAUVEGARDES DES JEUX SONT ICI), _a_supprimer/
sources/lanceur/                code Go du lanceur (module « ludotheque », purego vendoré)
sources/hatari/                 nos 4 patchs de Hatari 2.6.1 + LISEZMOI (recette de compilation)
outils/                         gen_catalogue.py, installer_images.py (+ images_tosec.json), tosec_tri.json
                                (Ludotheque.exe --trier), valide_windows.py,
                                captures.sh, test_ui.py, plan_captures.py
README.md                       page GitHub (installation depuis TOSEC) ; dépôt public fab48/ludotheque-st
```

## Compiler le lanceur

Go ≥ 1.24, sans CGO (purego pour X11 côté Linux) :

```bat
cd sources\lanceur
set CGO_ENABLED=0
go build -mod=vendor -ldflags "-s -w -H windowsgui" -o ..\..\Ludotheque.exe .
set GOOS=linux& go build -mod=vendor -ldflags "-s -w" -o ..\..\ludotheque .
```
(ou `build.bat` / `build.sh` à la racine). Hors ligne : `GOFLAGS=-mod=vendor GOPROXY=off GOSUMDB=off`.

Lancer pour tester : `Ludotheque.exe --fenetre` (navigateur en fenêtre),
`--sans-navigateur` (serveur seul, ouvrir http://127.0.0.1:8480/). Journal :
`runtime/ludotheque.log` (chaque lancement avec la commande Hatari complète).

## Architecture du lanceur (sources/lanceur)

- `main.go` : Config (config.json), routes HTTP sur 127.0.0.1:8480 :
  `/api/catalogue`, `/api/lancer`, `/api/arreter`, `/api/fin-web`, `/api/evenements` (SSE),
  `/api/admin` (F10), `/api/systeme` (quitter/éteindre), `/api/affichage` (écran des jeux),
  `/api/demos`, `/api/demo/*`, `/api/capture/retirer` ; fichiers `/ui/`, `/medias/`, `/reforged/`.
- `jeux.go` : catalogue (variantes, mode vrac = toutes les images de disques/, regroupées),
  `preparerHatari` (arguments + cfg générée + variables d'environnement ; **sous Windows,
  Hatari tourne dans le dossier du pack et ne reçoit que des chemins relatifs** : SDL lui
  passe ses arguments en UTF-8 mais il ouvre les fichiers en ANSI, un accent dans le dossier
  du pack lui faisait perdre le TOS ; dossier hors ASCII → nom court 8.3, `cheminCourt`),
  `disquePropre` (copie dans runtime/disques/ nommée `<empreinte du contenu>-<nom ≤ 40 car.>`,
  indépendante de l'emplacement du pack, anciennes copies reprises avec leurs sauvegardes :
  neutralise l'amorce « Little Green Desktop » en cassant la somme $1234, applique les
  `correctifs` octets → ex. IK+ sur STE, et copie toute image dont le chemin complet dépasse
  240 caractères : Hatari plafonne à 260 ; un .zip est extrait là, notre Hatari Windows n'a pas
  zlib), `sequenceDemarrage`
  (touche de menu de compil, encart des commandes), `Gestionnaire` (un jeu à la fois ;
  **arrêt = fermeture propre** WM_CLOSE / SIGTERM puis kill au bout de 4 s, sinon Hatari
  n'écrit pas les sauvegardes).
- `carte*.go` : encart des commandes (fenêtre GDI topmost sous Windows, X11 override-redirect
  sous Linux) : 3 s au lancement, puis Pause / Select (appui court) en jeu → pause + encart.
- `plateforme_windows.go` : Win32 par syscall (XInput, winmm, GetAsyncKeyState,
  keybd_event en scancodes, EnumWindows avec callbacks créés une seule fois).
  `plateforme_linux.go` : X11 via purego, /dev/input/js*, tube `--cmd-fifo` de Hatari.
- `demos.go` : veille (démos célèbres, bandeau, arrêt à la moindre action) et Play Demo
  Random (`/api/demo/demarrer` {"mode":"lecture"} : l'activité n'arrête pas ; Échap ou
  Select+Start reviennent, Espace ou tir → `/api/demo/suivante`, surveillés par
  `surveillerEntrees`).
- `disquettes.go` : jeu à plusieurs disquettes → runtime/hatari/disques.txt (fichiers de chaque
  disquette, lecture FAT12 des .st/.msa/.dim) pour le changement automatique de notre Hatari ;
  disquettes 1 et 2 toujours dans A et B au départ. Journal Hatari : runtime/hatari/hatari.log.
- `tri.go` : tri des noms TOSEC (une édition par jeu, incompatibles écartés, voir « Collection
  TOSEC ») ; sert au ménage, au mode vrac (titre, éditeur, année, disquettes « (Disk 1 of 3)(Intro) »
  regroupées, séquence ["espace"] ou ["N","espace"] si [t]) et à `Ludotheque.exe --trier`.
- `menage.go` : rangement de **disques/GAME** (et tout dossier game/games de disques/), où
  l'utilisateur dépose la collection TOSEC telle quelle ou en vrac. Au démarrage, si le contenu
  a changé (signature dans runtime/rangement.json), et par Fichier › Ménage des disquettes
  (`POST /api/menage` ; `GET` = état), **sans confirmation** : 1. installe les images absentes
  du catalogue d'après images_tosec.json (par nom de zip ou d'image, sinon par CRC dans tous
  les zips) ; 2. trie (tri.go) et efface tout le reste du dépôt : autres éditions, « déjà au
  catalogue », dossiers non-jeux de la collection (`autresTosec`, « Atari ST - Demos… »),
  zips sans disquette ; 3. active le mode vrac. Journal : runtime/menage-<date>.txt
  (INSTALLÉ / GARDÉ / EFFACÉ / ABSENT + raison). Jamais touché : hors du dépôt, nom non TOSEC,
  source d'une image du catalogue pas installée. À plat, démos et compils ne se distinguent
  pas des jeux (gardées comme jeux). L'interface affiche l'étape (évènement SSE « rangement »)
  puis le compte rendu, une fois (localStorage ludo-rangement-vu) ; /api/lancer refusé pendant.
- `affichage.go` : filtres d'écran → variables d'env lues par notre Hatari. En « lisse »,
  `--max-width 416 --max-height 276` : sinon Hatari double la basse résolution (832×552,
  blocs de 2×2) et le lissage linéaire ne se voit presque pas.
- `navigateur.go` : Chrome/Chromium/Edge/Firefox en kiosque, profil dans runtime/,
  `--disable-extensions` (Windows installait Adobe Acrobat dans le profil : chemins de 175 car.).
- **Le pack doit tourner de partout** (dossier profond, accents, `&`, crochets, `Łódź`) : tout
  chemin est relatif au pack (`chemin()`), aucun nom de fichier du pack ne dépasse ~45 car.
  Vérifié le 15/10/2026 en copiant le pack dans des dossiers piégeux (scratchpad).

Entrées : sortie de jeu = Échap maintenu 2 s, ou Select+Start 1 s (surveillés par le lanceur).
J1 = manette 1 ou flèches + Ctrl droit (Joystick1) ; J2 = manette 2 ou ZQSD + Ctrl gauche
(port 0, sauf jeux souris). F8 = capture (raccourci Hatari). F11 = accéléré manuel.

## Notre Hatari 2.6.1 (sources/hatari/*.patch)

Variables d'environnement posées par le lanceur :
- `HATARI_LUDO_FILTRE` net|lisse|entier, `HATARI_LUDO_LIGNES` 0..100, `HATARI_LUDO_43` 1
  (screen.c : rendu via texture cible ×k, lignes de balayage, 4:3).
- `HATARI_LUDO_TURBO=disque` (main.c/fdc.c/video.c/psg.c) : accéléré seulement pendant les
  chargements = moteur du lecteur allumé sans musique, ou décompactage (couleur 0 qui clignote
  sans écriture YM). `HATARI_LUDO_DEBUG=1` trace l'état par VBL.
- audio.c : silence pendant l'accéléré.
- `HATARI_LUDO_DISQUES=<liste>` (floppy.c/bios.c/cpu/newcpu.c) : changement automatique de
  disquette. Au trap #1 (Fopen, Fsfirst, Pexec, Dsetpath), un fichier absent de la disquette du
  lecteur mais présent sur une autre de la liste → celle-ci est insérée (échange A↔B si elle est
  dans l'autre lecteur), puis BIOS Mediach répond 2 (« changée ») : le TOS relit. Vérifié sur
  Gobliiins (DISK1.STK → disquette 2 en A, niveau 1 chargé). Sans effet sur les formats maison
  (`FORMAT_MAISON` de gen_catalogue.py : Cannon Fodder, Drakkhen, Gods, .stx…).

Recompiler : sources officielles https://github.com/hatari/hatari tag v2.6.1, appliquer les
patchs, CMake (recette exacte dans sources/hatari/LISEZMOI.txt). Windows : MSYS2 mingw64 (gcc,
cmake, ninja installés le 16/10) + SDL 2.30.9 officielle, sans zlib ni PNG. Linux : compiler sur
Ubuntu 22.04 (glibc 2.35) pour tourner sur Mint/Manjaro, livrer libSDL2-2.0.so.0 à côté : sans
machine Ubuntu, lancer le workflow « Hatari Linux » du dépôt (onglet Actions, `gh workflow run
hatari-linux.yml`) et récupérer l'artefact (`gh run download <id> -n hatari-linux`).

## Collection TOSEC

`C:\Atari_ST_TOSEC_2012_04_23\Atari ST [TOSEC]\` : la collection complète (32 737 zips, un par
disquette ; Games/[ST], Games/[STX] (Pasti, protégés), Demos, Compilations, TOS…). Les jeux et
démos retenus sont rangés dans `disques/tosec/` et `demos/` sous un **nom court** : `<id>.st`, ou
`<id>-<n>.st` à plusieurs disquettes (`nom_court` de gen_catalogue.py ; le nom TOSEC reste dans la
liste J et dans images_tosec.json, champ « ancien »). Choix d'une
édition : préférer `[cr Groupe]` sans `[b]` (mauvais dump), `(fr)` pour les jeux à texte, `[one
disk]` ; éviter `[t]` (trainer : « Y/N ? » à passer avec "sequence": ["N"]). Une STX est l'original
protégé (Hatari l'émule, mais `--fastfdc no` conseillé).

Candidats à ajouter : `Ludotheque.exe --trier "<dossier TOSEC>" [--tout] [--pd] [--sortie f.json]`
(tri.go, lecture seule, quelques secondes ; même résultat que l'ancien script Python, rangé dans
runtime/_a_supprimer/) écrit `outils/tosec_tri.json` : une édition par jeu (doublons [a]/cracks/langues/[ST]-[STX] écartés,
titres voisins même éditeur+année fusionnés, `aliasTitres` pour les fautes de TOSEC), hors jeux déjà
au catalogue (`--tout` pour les garder), et la liste des titres écartés avec leur raison
(Falcon/TT, monochrome, démo jouable, données/éditeur, extension (`reExtension`), incomplet, [b]
seulement, ZZZ-UNK). Le 16/10/2026 : 1 927 titres, 160 au catalogue, 1 595 candidats, 172 écartés.
Rangement testé le 16/10 sur des packs vides (scratchpad, LUDO_DIR) avec 1 130 zips de TOSEC
(Games, Compilations, TOS, Demos, Applications), rangés ou à plat dans disques/GAME : les 216 images
du catalogue installées, ~200 jeux gardés, le reste effacé, fichier perso gardé ; Operation Wolf
lancé depuis son zip.

Installation des images : déposer la collection dans disques/GAME (menage.go s'en charge), ou
`outils/installer_images.py "<dossier TOSEC>"`
(ou « Installer les images TOSEC.bat ») extrait les 216 images du catalogue (disques + TOS)
d'après `outils/images_tosec.json` (zip, membre, CRC32, taille), sans jamais écraser une image
présente ; une image installée sous son ancien nom TOSEC (v1.0) est renommée, avec ses copies
de runtime/disques (`--renommer` : sans la collection). **Après tout ajout au catalogue** : `python outils/installer_images.py --carte "<TOSEC>"`
refait le manifeste (recherche par CRC ; une image modifiée par des sauvegardes garde sa source
notée). Le lanceur cache du bac les jeux dont une image ou le TOS manque (`completer` vérifie
aussi le TOS ; l'interface compte les « absentes »).

## Catalogue

`outils/gen_catalogue.py` génère catalogue.json : liste `J` (id, titre, éditeur, année,
genre, joueurs, disques, options machine/tos/touche/note/reforged), `REGLAGES` (machine, tos,
options Hatari par jeu), `AUTO` (programme lancé par --auto : jeux qui s'arrêtent sur le bureau
GEM), `TOUCHES` (ancienne forme : touche du menu de compil, pavé par défaut, `c` = rangée des
chiffres, `esp` = Espace), `SEQ` (« sequence » : touches pressées à tour de rôle au démarrage —
espace, entree, F1…F10, pave1…9, 1…0, lettres ; tout jeu TOSEC sans réglage reçoit ["espace"]),
`DUREE` (« touche_duree », 5 s par défaut), `DELAI` (« touche_delai »), `SOURIS`, `AIDE`
(« Touche : action · … », affiché en deux colonnes), `INACTIFS` (hors du bac), correctifs IK+,
`MHZ16_CONSEILLE` (« mhz16 »: « conseille » : case « Processeur 16 MHz » de la fiche sans alerte ;
les autres jeux affichent une alerte). La case (clic, F6, Select manette) est gardée par jeu dans
le navigateur (ludo-prefs) ; `/api/lancer` {"mhz16": true} ajoute `--cpuclock 16` après `--machine`.
`python outils/gen_catalogue.py --garder` reprend les cases cochées (F10) et le mode vrac de
l'ancien catalogue.json ; sans `--garder`, tout est coché sauf `INACTIFS`.
Après une modif : `python outils/valide_windows.py <id> …` (voir plus bas).

Jeux réglés à part : Lotus et Populous en TOS 2.06 (Populous en ST), IK+ correctif STE
(`41f900fc0000`→`41f900030000`), Barbarian F1/F2 pour démarrer.

## config.json (principales clés)

port, navigateur, kiosque, plein_ecran_jeux, sortie_echap_s, sortie_manette_s,
boutons_manette, joystick (auto|manette|clavier), tir_clavier(_j2), touche_capture,
touche_menu_auto, demos_apres_min, duree_demo_min, nettoyer_amorce, carte_au_chargement,
turbo (disque|debut|non), turbo_chargement_s (mode debut), ecran_filtre, ecran_lignes,
ecran_4_3, ecran_hz (50 par défaut : écran principal passé à 50 Hz, ou 100, pendant Hatari en plein
écran, jeux et démos, puis remis 1,5 s après ; 0 = ne pas toucher ; menu Options › « Écran à 50 Hz » ;
`frequence*.go`, ChangeDisplaySettingsEx CDS_FULLSCREEN ; pas encore sous Linux).

## Tester sans écran (Windows)

`python outils/valide_windows.py [id …]` (Pillow requis) : chaque jeu du catalogue.json comme
le lanceur le lancerait (arguments, turbo « disque », touches de démarrage), Hatari en fenêtre
hors écran sans prendre le focus (touches par PostMessage), 8 en parallèle, une capture toutes
les 250 VBL via le débogueur (`b VBL = N :trace :quiet :once :file x.ini`, `screenshot f.bmp` —
le PNG n'est pas compilé dans notre Hatari ; sans `:once`, capture à chaque instruction !).
Planches : runtime/tests/planche-NN.png. `VMAX=4500` (VBL) pour regarder plus loin, `MHZ=16`
(résultats dans runtime/tests-16mhz). **Tout le catalogue = des centaines de Hatari : demander
à Fab avant**, il utilise le PC pendant ce temps.

## Tester sans écran (Linux)

- `outils/captures.sh <id> <tos> <machine> <disqueA> <disqueB|-> <auto|-> "<s:scancodeST> …"`
  : Hatari sans affichage + tube de commandes, une capture toutes les 3 s (DUREE=…,
  EXTRA="--memsize 512", NOLGD=1). Scancodes ST : pavé 1-9 = 0x6d,0x6e,0x6f,0x6a…, F1 = 0x3b,
  Espace = 0x39, « clic » = clic gauche.
- `outils/test_ui.py` : Playwright contre le lanceur lancé avec `--sans-navigateur`
  (cliquer d'abord l'écran « allumer »).
- Plein écran / filtres : Xvfb + `import -window root`.

## État / à faire

- 13/10/2026 (Claude Code, sous Windows) : collection TOSEC branchée. Catalogue = 162 jeux
  (63 d'avant + 99 classiques ajoutés), tous dans le bac sauf `INACTIFS` ; jeux manquants de
  la liste trouvés (Lemmings, Speedball 2, Another World VF, Arkanoid II, Strider, Prince of
  Persia une disquette) ; compils Automation qui bloquaient remplacées par des images TOSEC
  (Chase HQ, Rainbow Islands, Midwinter, Kick Off 2 VF, New Zealand Story, Turrican II,
  Pirates!, Castle Master, Barbarian II, Monkey Island VF). 35 démos célèbres + Play Demo Random.
- Vérifié sous Windows le 13/10 : turbo « disque », fermeture propre (WM_CLOSE), touches de
  démarrage, veille et Play Demo Random. Corrigé : la veille s'arrêtait aussitôt lancée (l'Alt
  simulé de forcerPremierPlan passait pour une action de l'utilisateur → `entreeSimulee`).
- Disquette originale de Stunt Car Racer pour Reforged HD : disques/Stunt_Car_Racer.st = la
  seule image TOSEC qui contient GAME.PUT (marquée [b2], « mauvais dump ») — à essayer.
- Populous : plantage « double bus error » à l'écran LOAD GAME (copie Automation 081) ;
  passé en ST + TOS 2.06 sans pouvoir le reproduire → à retester.
- Stunt Car Racer : ~3 s d'écran blanc non accéléré (rien de détecté).
- Protections des cracks à passer à la main (rappel dans l'aide de la fiche) : Monkey Island
  (question du pirate), Operation Stealth, Croisière pour un cadavre, Starglider 2, Gunship.
- Pas encore vérifié sous Windows : encart Pause/Select, filtres d'écran (vus OK sur une
  capture de Fab).
- medias/<id>/1.png des 135 jeux qui n'en avaient pas : capture automatique (la plus colorée
  de la validation) ; quelques-unes montrent une protection ou un texte → F8 en jeu les remplace.
- IK+ Reforged à déposer. Idées évoquées : la même chose pour DOSBox plus tard.
- 14-15/10/2026, v1.1 : case « Processeur 16 MHz » (F6, Select), écran à 50 Hz en jeu (testé :
  60 → 50 → 60 Hz ; une fois, le Chrome de la ludothèque s'est fermé 4 s après le retour à
  60 Hz sans explication, à surveiller), ascenseur du bac à glisser, délai de 30 s sur
  /api/lancer (plus d'interface figée sur « Lecture piste 79 »), noms courts des disquettes et
  des démos, chemins relatifs pour Hatari (accents), copies runtime/disques par contenu.
  Retour d'Axel (v1.0 bloquée sur « piste 79 » sur C:, OK sur D:) : cause exacte inconnue.
- 16/10/2026 : changement automatique de disquette (patch ludotheque-disques, disquettes.go),
  Hatari Windows recompilé ; 23 jeux concernés, note de fiche « Disquettes changées
  automatiquement ». Hatari Linux recompilé avec ce patch le même jour (GitHub Actions,
  `.github/workflows/hatari-linux.yml`, pas testé sur un vrai Linux). Formats maison (10 jeux) : encore F12, piste possible =
  rotation quand le jeu relit en boucle les mêmes secteurs (fdc.c). valide_windows.py : captures
  rétablies (le débogueur coupait les chemins à l'espace de « Hatari Loader »).
- 16/10/2026 : recherche dans le bac et le panneau F10 (ui/index.html, barre sous #infos) : taper
  un nom filtre aussitôt (tous les mots, dans titre/éditeur/année/genre, sans accents), Échap
  efface ; boutons # A…Z pour sauter à une lettre. Les lettres tapées ne sautent plus à la
  lettre (elles cherchent). Tout/Rien du panneau = les jeux montrés. La veille vide la recherche.
