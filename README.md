
source des jeux et demo dispo ici : 

https://archive.org/details/Atari_ST_TOSEC_2012_04_23

https://mega.nz/folder/vYE2lZob#nFp7XGJYF0ycnJ6cj3my4A

# Ludothèque ST

Une ludothèque Atari ST au look du bureau GEM : un bac de disquettes animé dans le
navigateur, une fiche par jeu (captures, commandes, versions), et l'émulateur
[Hatari](https://hatari.tuxfamily.org/) qui démarre le jeu en plein écran. Pensée pour
une borne ou un salon : clavier, souris ou manette, sortie d'un jeu par Échap maintenu
ou Select + Start.

- **162 jeux au catalogue** (Xenon 2, Dungeon Master, Another World, Monkey Island,
  Lemmings, Kick Off 2, Populous, Turrican, Captain Blood…), réglés et testés un par un :
  bon TOS, bonne machine (STF ou STE), touches des menus de compil et des intros de crack
  pressées toutes seules.
- **35 démos célèbres** de la scène ST, en écran de veille ou en **Play Demo Random**
  (F7).
- **Hatari 2.6.1 modifié** : chargements accélérés seulement quand le lecteur tourne,
  filtres d'écran, format 4:3.
- Versions « reforged » : Stunt Car Racer en HD (natif) et en réplique web, Shufflepuck
  Café web ; IK+ Reforged se dépose dans `reforged/ikplus/`.
- **100 % portable** : Windows et Linux (Mint, Manjaro…), rien à installer pour jouer.

> Le dépôt ne contient **ni images de disquettes ni ROM TOS** (droits d'auteur) : elles
> s'installent en une commande depuis la collection TOSEC, voir plus bas.

## Installation

### 1. Récupérer la ludothèque

Téléchargez le dépôt (bouton *Code › Download ZIP*) ou clonez-le :

```bash
git clone https://github.com/fab48/ludotheque-st.git
```

### 2. Compiler le lanceur

Le lanceur est un petit programme Go, sans dépendance (Go 1.24 ou plus récent,
`winget install GoLang.Go` sous Windows) :

```bat
build.bat
```

```bash
./build.sh
```

Cela produit `Ludotheque.exe` (Windows) et `ludotheque` (Linux) à la racine. Hatari
(Windows et Linux) est déjà dans `emulateurs/`.

### 3. Installer les images de disquettes et les TOS

Les jeux du catalogue viennent tous de la collection **TOSEC Atari ST 2012-04-23**
(dossier « Atari ST [TOSEC] », environ 32 700 zips, un par disquette). Une fois cette
collection décompressée quelque part sur votre disque :

- **Windows** : glissez le dossier de la collection sur
  **`Installer les images TOSEC.bat`** (ou lancez-le et tapez le chemin).
- **Linux** :

  ```bash
  python3 outils/installer_images.py "/chemin/vers/Atari ST [TOSEC]"
  ```

Le script (Python 3, rien d'autre) extrait **216 images** : les disquettes dans
`disques/` et `disques/tosec/`, les TOS 1.02, 1.04, 1.62 et 2.06 français dans `tos/`.
Chaque fichier est contrôlé (CRC32 et taille) d'après `outils/images_tosec.json`. Il ne
remplace jamais une disquette déjà présente : elle peut contenir vos sauvegardes. Si
votre collection est rangée autrement, les zips sont retrouvés par leur nom, puis par
leur contenu. Relancer le script ne fait que compléter ce qui manque.

**Le bac s'adapte aux fichiers présents** : un jeu dont l'image ou le TOS manque n'y
apparaît pas. La barre d'info indique combien attendent leurs fichiers, et le panneau
de contrôle (F10) les liste comme « absent ».

### 4. Lancer

| | |
|---|---|
| Windows | double-clic sur `Ludotheque.exe` |
| Linux | `bash ludotheque.sh` |
| En fenêtre | `Ludotheque.exe --fenetre` / `./ludotheque.sh --fenetre` |

Le navigateur s'ouvre en plein écran (Chrome, Chromium, Edge ou Firefox). Démarrage
automatique avec la session : `Demarrage automatique - activer.bat` (Windows),
`bash installer-linux.sh` (Linux, `--kiosque` pour une session dédiée).

## Vos propres disquettes

Déposez des images `.st`, `.msa`, `.stx` ou `.zip` dans `disques/` et activez le
**mode vrac** (F10) : elles apparaissent dans le bac, regroupées par jeu. Pour leur
donner une fiche (titre, éditeur, commandes, TOS…), ajoutez une entrée dans
`outils/gen_catalogue.py` puis :

```bash
python outils/gen_catalogue.py --garder
```

(`--garder` conserve vos choix faits avec F10.) Un jeu tiré de TOSEC peut ensuite être
noté pour l'installation automatique : `python outils/installer_images.py --carte
"<dossier TOSEC>"`.

Démos : déposez-les dans `demos/` ; titre et texte du bandeau dans `demos.json`.

## Commandes

| | Clavier | Manette |
|---|---|---|
| Choisir une disquette | ← → | croix, stick |
| Charger | Entrée | A |
| Menus | F9 ou Tab | Start |
| Panneau de contrôle (bac, mode vrac) | F10 | Y |
| Play Demo Random | F7 | |
| **Quitter un jeu** | **Échap maintenu 2 s** | **Select + Start 1 s** |
| Commandes du jeu / pause | Pause | Select |
| Capture pour la fiche | F8 | |

En jeu, le joueur 1 joue à la manette 1 (ou flèches + Ctrl droit) et le joueur 2 à la
manette 2 (ou Z Q S D + Ctrl gauche). Le mode d'emploi complet est dans
[LISEZMOI.txt](LISEZMOI.txt).

## Contenu du dépôt

```
ui/                    l'interface (une page HTML/CSS/JS)
sources/lanceur/       le lanceur (Go)
sources/hatari/        nos correctifs de Hatari 2.6.1
emulateurs/            Hatari 2.6.1 modifié (Windows, Linux) et Hatari 1.8
catalogue.json         les jeux (généré par outils/gen_catalogue.py)
demos.json, demos/     les démos
medias/                les captures des fiches
reforged/              versions modernisées (Stunt Car Racer, Shufflepuck, IK+)
outils/                catalogue, installation des images, tests
disques/, tos/         vides : à remplir avec outils/installer_images.py
```

## Licences

Hatari est sous licence GPL v2 (sources : <https://github.com/hatari/hatari>, nos
correctifs dans `sources/hatari/`). Les démos sont des productions gratuites de la scène
Atari ST. Les jeux et les TOS restent la propriété de leurs auteurs : ils ne sont pas
fournis ici.
