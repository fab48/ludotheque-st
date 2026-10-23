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
medias/<id>/                    captures des fiches (1.png, 2.png fournies ; grab*.png = AltGr+G en jeu)
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
exports/                        zips exportés (Fichier › Exporter en zip, export.go)
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
  `/api/catalogue` (`?vrac=favoris` : sans trier le vrac, seulement ses favoris), `/api/vrac`
  (tout le vrac, demandé ensuite par l'interface), `/api/lancer`, `/api/arreter`, `/api/fin-web`, `/api/evenements` (SSE),
  `/api/admin` (F10), `/api/systeme` (quitter/éteindre), `/api/affichage` (écran des jeux),
  `/api/demos`, `/api/demo/*`, `/api/capture/retirer`, `/api/menage`, `/api/exporter` ; fichiers `/ui/`, `/medias/`, `/reforged/`.
- `jeux.go` : catalogue (variantes, mode vrac = toutes les images de disques/, regroupées ;
  ★ d'un jeu du vrac dans `favoris_vrac` de catalogue.json : 1re disquette → ses disquettes ;
  case « Touches au démarrage » décochée : `sans_touches` du jeu, `sans_touches_vrac` pour le vrac),
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
  `imageLisible` (zip avec une disquette ?) garde ses réponses dans runtime/lisibles.json (chemin
  relatif, taille, date à la seconde) : sans lui, chaque démarrage rouvrait tous les zips du vrac.
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
- `export.go` : Fichier › Exporter en zip (`POST /api/exporter` {"mode": "favoris"|"bac",
  "adultes"}, évènements SSE « export ») ou `Ludotheque.exe --exporter favoris|bac [--adultes]
  [--sortie f.zip]` : zip prêt à l'emploi Windows + Linux dans **exports/** (ignoré par git),
  dossier `ludotheque-st/` : les 2 lanceurs, ui, emulateurs, tos, demos, config.json, catalogue.json
  réduit aux jeux choisis (favoris = cochés et ★, et les ★ du vrac ; bac = cochés + vrac s'il est affiché ; jeux
  jouables seulement), leurs images, medias, dossiers Reforged et sauvegardes (copie de
  runtime/disques retrouvée par `prefixeCopie`/`ancienneCopie`, comme `disquePropre`). Vrac de
  disques/GAME → disques/vrac/ (le ménage du nouveau pack n'y touche pas). Mode 755 pour les
  fichiers sans extension et les .sh. Le zip contient aussi runtime/lisibles.json (réponses
  d'`imageLisible` pour ses zips) : le nouveau pack ne les rouvre pas à son premier démarrage
  (dates gardées à la seconde par unzip sous Linux ; l'Explorateur de Windows les arrondit, les
  zips sont alors relus une fois). Testé le 18/10 : favoris 12 jeux 73 Mo en 2 s ; bac
  163 + 2 157 jeux, 960 Mo en 28 s ; les deux dézippés et relancés (dossier avec accent) : tous
  les jeux retrouvés.
- `affichage.go` : filtres d'écran → variables d'env lues par notre Hatari. En « lisse »,
  `--max-width 416 --max-height 276` : sinon Hatari double la basse résolution (832×552,
  blocs de 2×2) et le lissage linéaire ne se voit presque pas.
  Bords de l'écran ST rognés (`argsAffichage`) : Hatari montre 416×276 (48 points à gauche
  et à droite, 29 lignes en haut, 47 en bas) pour une image de 320×200, et ces bords noirs
  laissaient au jeu 72 % de la hauteur (Shufflepuck). `--max-width 640 --max-height 424`
  (`320 212` en « lisse ») : 320×212, mêmes proportions, 4:3 toujours juste. Gardés pour les
  démos, les jeux `"bords": true` (`BORDS` de gen_catalogue.py, overscan) et tous si `ecran_bords`.
- `navigateur.go` : Chrome/Chromium/Edge/Firefox en kiosque, profil dans runtime/,
  `--disable-extensions` (Windows installait Adobe Acrobat dans le profil : chemins de 175 car.).
- **Le pack doit tourner de partout** (dossier profond, accents, `&`, crochets, `Łódź`) : tout
  chemin est relatif au pack (`chemin()`), aucun nom de fichier du pack ne dépasse ~45 car.
  Vérifié le 15/10/2026 en copiant le pack dans des dossiers piégeux (scratchpad).

Entrées : sortie de jeu = Échap maintenu 2 s, ou Select+Start 1 s (surveillés par le lanceur).
J1 = manette 1 ou flèches + Ctrl droit (Joystick1) ; J2 = manette 2 ou ZQSD + Ctrl gauche
(port 0, sauf jeux souris). AltGr+G = capture (raccourci Hatari ; plus F8, que Mortevielle
attend : `touche_capture` de config.json n'est jamais une touche F1…F10). F11 = accéléré manuel.

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
un jeu = un titre (`cleTitre`, « + » compte) et ses images qui ont un jeton en commun (`jetonsTosec` :
éditeur, ou année d'un jeu du commerce), `aliasTitres` pour les fautes de TOSEC), hors jeux déjà
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
les autres jeux affichent une alerte), `FAVORIS` (« favori »: true). La case (clic, F6, Select manette) est gardée par jeu dans
le navigateur (ludo-prefs) ; `/api/lancer` {"mhz16": true} ajoute `--cpuclock 16` après `--machine`.
`python outils/gen_catalogue.py --garder` reprend les cases cochées (F10), les favoris et le mode vrac de
l'ancien catalogue.json ; sans `--garder`, tout est coché sauf `INACTIFS`.
Après une modif : `python outils/valide_windows.py <id> …` (voir plus bas).

Jeux réglés à part : Lotus et Populous en TOS 2.06 (Populous en ST), IK+ correctif STE
(`41f900fc0000`→`41f900030000`), Barbarian F1/F2 pour démarrer.

## config.json (principales clés)

port, navigateur, kiosque, plein_ecran_jeux, sortie_echap_s, sortie_manette_s,
boutons_manette, joystick (auto|manette|clavier), tir_clavier(_j2), touche_capture,
touche_menu_auto, demos_apres_min, duree_demo_min, nettoyer_amorce, carte_au_chargement,
turbo (disque|debut|non), turbo_chargement_s (mode debut), ecran_filtre, ecran_lignes,
ecran_4_3, ecran_bords (false : bords de l'écran ST rognés, menu Options › « Jeux : bords de
l'écran ST »), ecran_hz (50 par défaut : écran principal passé à 50 Hz, ou 100, pendant Hatari en plein
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
  de la validation) ; quelques-unes montrent une protection ou un texte → AltGr+G en jeu les remplace.
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
- 16/10/2026 : favoris. Le « panneau de contrôle » s'appelle **Catalogue** dans l'interface (F10,
  menu Bureau). Étoile par jeu (clic, Espace/X) → « favori » dans catalogue.json (`/api/admin`
  {"favoris": {id: bool}}). Filtre du bac « Favoris seulement » (F4, bouton ★ de la barre,
  Affichage, manette LT = bouton 6), gardé dans ludo-prefs, mis par défaut ; la recherche
  ignore le filtre (fouille tout le bac). 16 favoris de départ (`FAVORIS` de gen_catalogue.py :
  les 12 du 16/10 + 4 ajoutés le 19/10).
- 16/10/2026 : 15 jeux cultes ajoutés (177 au catalogue) : Starglider, The Sentinel, Elite, Head Over
  Heels, Zool, Super Off Road, Lotus Turbo Challenge 2, Operation Wolf, Rocket Ranger, Batman The Movie,
  Leisure Suit Larry, Gobliins 2 (v1.02 [cr ICS] : l'édition FR [cr ICS] a un répertoire illisible,
  rangée dans runtime/_a_supprimer/), Ishar 2 ([cr Elite], anglais), Pro Tennis Tour (Great Courts),
  Time Bandit. Réglages devinés d'après le contenu des disquettes (AUTO, STF pour les 1985-87,
  trainers « N »). Ce sont des suggestions de Claude, pas la liste de Fab (ses jeux = les favoris).
  Lancés le 19/10 (valide_windows.py) : arrivent à l'écran titre, au menu ou à l'intro : Sentinel
  (« Landscape number? »), Head Over Heels (menu, F1), Zool, Operation Wolf, Larry (avertissement,
  Entrée), Gobliins 2 et Ishar 2 (intro), Great Courts, Time Bandit (1 ou 2) ; Super Off Road corrigé
  (voir plus bas). **Encore bloqués, pas corrigés** : Starglider (écran noir, même en STE ou TOS 1.02 :
  image TOSEC sans crack), Elite (STE : erreur de bus en $FC2F1E, la ROM du TOS 1.x ; en STF : protection
  Novella, un mot du manuel), Lotus Turbo Challenge 2 (menu Cynix : « 1 », puis protection du manuel),
  Rocket Ranger (« File GMTN not found on Reel 1 », aussi en STF), Batman The Movie (bureau GEM en STE,
  écran blanc en STF).
- 16/10/2026 : le mode vrac trie comme le ménage (`ecartesVrac`, jeux.go) sans rien effacer :
  une édition par jeu, rien de ce que le catalogue a déjà, titres écartés cachés. Visait les
  images TOSEC posées à la racine de disques/ (5 Vroom…), que le ménage ne touche pas.
  Disquettes de l'édition gardée regroupées même si leurs noms diffèrent ((King1)/(King2),
  [!] sur une seule), compils Automation A_483_A…D = un jeu. Vrac : 2 251 → 2 196 fiches.
  `imageLisible` garde ses réponses (zip inchangé) : /api/catalogue ~150 ms.
- 16/10/2026 : jeux pour adultes du vrac cachés. `estAdulte` (jeux.go : titres « strip », « sex », « fuck »,
  Emmanuelle, Geisha, Teenage Queen, Hollywood Poker, Piss Flaps, Blue Angel 69…, éditeurs Free Spirit et Sexy Technologies) pose
  `adulte` ; un jeu du catalogue peut aussi porter `"adulte": true`. L'interface ne les montre
  qu'après le Konami code tapé au clavier sur le bureau (↑ ↑ ↓ ↓ ← → ← → B A, `suivreKonami`) ;
  le même code les recache, la veille aussi, et le filtre Favoris les laisse passer. Disques
  locaux scannés le 16/10 : 24 titres (Geisha, Hollywood Poker Pro, Piss Flaps 1-2 ajoutés à la
  main) ; laissés visibles faute de titre explicite : Leisure Suit Larry, Bootiful Babe, Datequest,
  Manga Paradise, La Belle Zohra.
- 19/10/2026, v1.3 : passe de relecture (lanceur, interface, ménage). **Le ménage effaçait des jeux
  distincts** (journal du 16/10 : War Zone 1986, Barbarian II de Psygnosis, International Karate 1986,
  Indiana Jones l'aventure, Night Breed, Spaceball 1988…, et des jeux à plusieurs disquettes complets
  comme Garfield Winter's Tail) : clé = titre seul, fusion « Titre - sous-titre ». Désormais
  (tri.go) : jetons éditeur/année, plus de fusion des sous-titres, 3e passe pour regrouper les
  disquettes (titre, année, éditeur, crack), « disquettes manquantes » gardées, nom non TOSEC
  toujours gardé, .dim accepté, image installée via un .part. Rejoué sur les 7 081 noms du
  journal : 117 fichiers de plus gardés, 7 de plus effacés (meilleure édition reconnue complète).
  Reste : deux jeux du commerce de même titre et même année (Barbarian de Palace et de Psygnosis,
  1987) sont confondus ; quelques doublons de jeux du catalogue restent dans le vrac quand TOSEC
  écrit le studio au lieu de l'éditeur (Rick Dangerous II Micro Style / Core Design). Les jeux
  décochés (F10) comptent toujours « au catalogue » (leurs éditions TOSEC sont effacées).
  Aussi : routes /api refusées aux autres pages web (Origin/Host, `gardien`), deux Hatari possibles
  sur double lancement (état « lancement »), pas de jeu par-dessus une démo, `medias/../` refusé,
  cache de version Hatari protégé (Linux), config.json pas réécrit s'il est illisible,
  `ancienneCopie` compare le contenu (une copie d'un autre jeu au même nom était reprise),
  ménage et export exclusifs. Interface : jeu arrêté pendant « Lecture piste » (écran « en jeu »
  bloqué), touche de réveil de la veille exécutée (⏎ lançait un jeu), vrac vide (`null`) qui
  cassait le bac, compte rendu du rangement perdu pendant l'allumage, Espace/Suppr gardés par le
  champ de recherche, alerte par-dessus F10, demos_apres_min 0, Select+Start relâchés un à un.
  La collection TOSEC n'est plus dans C:\Atari_ST_TOSEC_2012_04_23 (seul reste disques/Games,
  déjà passé au ménage du 16/10).
- 19/10/2026 : les jeux de Fab validés dans Hatari (valide_windows.py) : les 12 favoris, plus Arkanoid 2,
  Bubble Bobble, Battle Chess et Super Off Road (demandés en favoris le 16/10, demande interrompue :
  ajoutés à `FAVORIS` le 19/10, 16 favoris). Tous arrivent à l'écran titre, au menu ou en démo.
  Deux écrans demandent une disquette qui est déjà là : Shufflepuck Café (« Insérer le disque
  shufflepuck1 dans le lecteur 0 », vers 16 s : une touche) et Super Off Road (« Please insert DATA
  disk… press FIRE », vers 45 s : tir ou Espace). Vérifié : passés, on arrive au bar / à la course.
  Pas d'appuis automatiques jusque-là (une touche toutes les 250 ms pendant 30 à 60 s gêne le joueur) :
  l'aide de la fiche le dit. Shufflepuck : séquence ["entree"] pour l'intro du crack (Espace = menu).
- 22/10/2026 : export « bac » lent sur deux PC Mint (et menu de Hatari au lancement d'un jeu,
  cause pas encore trouvée : TOS bon, puisque OK dans ce menu lance le jeu ; journal des touches
  demandé à Fab avec `"hatari_options": ["--trace", "keymap", "--trace-file",
  "runtime/hatari/touches.txt"]`). Lenteurs liées au nombre de jeux, mesurées sur un pack de
  177 + 1 998 zips (processeur bridé ×6 pour l'interface) : le premier /api/catalogue de chaque
  démarrage ouvrait les 1 998 zips (`imageLisible`, cache en mémoire seulement) → gardé dans
  runtime/lisibles.json, aussi écrit par l'export (0 zip ouvert au 1er démarrage d'un export
  dézippé) ; le bac créait un bouton par disquette (1 743 boutons, 12 861 éléments) → seulement
  les ±10 autour de la sélection (`boutonsBac`, créés et retirés par `placer`) : bac complet
  899 → 109 ms, une lettre de recherche 636 → 122 ms, rendu identique au pixel près.
  Hatari sans pilote graphique (llvmpipe) : « pixels nets » coûte 90 % d'un cœur contre 54 %
  en rendu simple, et divise par deux l'accéléré des chargements (×3,9 au lieu de ×8) ; le
  facteur k de `Ludo_Rendu` est calculé sur toute la largeur de l'écran au lieu de la zone 4:3
  (k = 3 au lieu de 2 en 1080p) : à corriger dans le patch si les vieux PC rament en jeu.
- 22/10/2026 (suite) : le bac s'affiche d'abord avec le catalogue et les favoris
  (`chargerCatalogue` → /api/catalogue?vrac=favoris), le tri de tout le vrac suit en arrière-plan
  (`chargerVrac` → /api/vrac, « vrac : tri en cours… » dans la barre) et n'est redemandé qu'après
  un ménage (`vracGen` écarte une réponse périmée). Bouton ★ sur la fiche (F2, manette RT =
  bouton 7, F2 aussi dans le panneau F10 ; pas F5, qui rechargeait la page : l'interface bloque
  F5 et Ctrl+R, Ctrl+F5 recharge) : catalogue et vrac, enregistré aussitôt par /api/admin ;
  étoile dorée sur le boîtier des disquettes favorites ; sous le filtre Favoris, la disquette
  retirée s'en va et la sélection passe à sa voisine. L'image originale d'une version native
  (Stunt_Car_Racer.st) ne compte plus comme un jeu du vrac. Formats maison (Cannon Fodder…) :
  toujours F12 › Floppy disks pour changer de disquette (signalé « HS » par Fab sous Mint).
- 22/10/2026 (suite) : « cliiiii » sans fin dans Rick Dangerous (clic clavier du TOS répété) :
  les touches de démarrage (Espace pendant 5 s) étaient tenues 120 ms (Windows) ou 250 ms
  (Linux, tube) en temps réel, soit 1 à 2 s pour le ST pendant l'accéléré ; le TOS répétait la
  touche et, si le jeu prenait le clavier avant le relâché, ne la relâchait jamais. Désormais
  `--fast-forward-key-repeat no` (Hatari relâche chaque touche au bout d'une image pendant
  l'accéléré) et appui de 30 ms par le tube sous Linux (pas concerné par cette option). Pas
  vérifié sur le jeu lui-même (pas d'image ici). Mortevielle : F8 (ses menus) était la touche de
  capture de Hatari → capture sur AltGr+G (raccourci d'origine de Hatari), `toucheCapture`
  écarte F1…F10 même si config.json les demande ; F8 vu arriver au ST (code 0x42).
- 22/10/2026 (suite) : Sherman M4 (vrac) choisissait ses menus tout seul, en continu : touche
  bloquée de la version d'avant (voir clic sans fin ci-dessus). En plus : case « Touches au
  démarrage » sur la fiche (clic ; montrée quand le jeu a des touches de démarrage, `touchesDemarrage`
  de l'interface = celle du lanceur) pour couper l'Espace de l'intro sur un jeu qui le prend pour un
  choix de menu ; enregistrée par /api/admin {"touches": {id: bool}}, suivie par l'export et
  `gen_catalogue.py --garder`. Sous Linux, relâché seul des touches de démarrage à la fin des
  appuis et 3 s après (`relacherTouche`, tube) : débloque le TOS si le vrai relâché s'est perdu ;
  rien sous Windows (Hatari ignore un relâché sans appui).
- 23/10/2026 : Shufflepuck Reforged sans son sur le fixe (Windows) : la page joue bien ses sons
  dans Chromium (signal mesuré dans le cadre de la ludothèque), cause pas trouvée, Fab laisse
  tomber. Bandes noires autour des jeux en plein écran = les bords de l'écran du ST que Hatari
  affiche (cadre de couleur 0) : rognés par défaut (voir `affichage.go`), image du jeu ×1,3 en
  largeur et en hauteur, ×2 en « pixels entiers ». Vérifié sous Xvfb (EmuTOS, 1920×1080) ; pas
  encore sur Shufflepuck lui-même. `BORDS` (Lethal Xcess, Wings of Death, Enchanted Land) est
  deviné et ne vaut qu'après `gen_catalogue.py --garder` (catalogue.json pas régénéré ici).
