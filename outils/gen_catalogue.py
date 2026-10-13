#!/usr/bin/env python3
"""Génère catalogue.json pour la ludothèque à partir d'une liste éditable.

Chaque ligne : id, titre, éditeur, année, genre, joueurs, [disques...], options.
Les couleurs des disquettes et des étiquettes sont tirées de l'id (stables).

  python gen_catalogue.py [catalogue.json] [--garder]
--garder : reprend les cases cochées (F10) et le mode vrac de l'ancien catalogue
pour les jeux qui y étaient déjà (sinon : ACTIFS ci-dessous).

Les images « tosec/… » viennent de la collection TOSEC 2012 (disques/tosec/).
"""
import json, hashlib, os, sys

CORPS = ['#1c1c1f', '#2b4f8c', '#9c2b25', '#6b6e73', '#26573a', '#1c1c1f', '#3b3f8f']
BANDES = ['#e8b230', '#c0392b', '#2f9e8f', '#8e44ad', '#3c9a3c', '#d35400', '#2e86de',
          '#f39c12', '#27ae60', '#95a5a6', '#e84393', '#16a085']
POLICES = ['manuscrit', 'machine']

J = [
 # id, titre, editeur, annee, genre, joueurs, disques, extra
 ('stunt-car-racer', 'Stunt Car Racer', 'Micro Style', 1989, 'Course', 1, ['A_117_stuntcarracer.ST'],
  {'auteur': 'Geoff Crammond', 'reforged': [
     {'type': 'natif', 'nom': 'Reforged HD', 'desc': 'natif, 1080p',
      'windows': ['reforged/stuntcar-natif/scr.exe', '{disque}', '--fullscreen', '--hd'],
      'linux': ['reforged/stuntcar-natif/scr', '{disque}', '--fullscreen', '--hd'],
      'disque': 'disques/Stunt_Car_Racer.st'},
     {'type': 'web', 'nom': 'Réplique web', 'desc': 'navigateur', 'url': 'reforged/stuntcar-web/index.html'}]}),
 ('shufflepuck-cafe', 'Shufflepuck Café', 'Brøderbund', 1989, 'Bar', 1,
  ['Shufflepuck Cafe (1989)(Broderbund)[cr Replicants][one disk].st'],
  {'auteur': 'Christopher Gross', 'reforged': [
     {'type': 'web', 'nom': 'Reforged', 'desc': 'fichier unique', 'url': 'reforged/shufflepuck/shufflepuck.html'}]}),
 ('xenon', 'Xenon', 'Melbourne House', 1988, 'Shoot them up', 1, ['A_028_xenon.ST'], {'auteur': 'The Bitmap Brothers'}),
 ('barbarian', 'Barbarian', 'Palace Software', 1987, 'Combat', 2, ['A_029_barbarian.ST'], {'touche': '3'}),
 ('barbarian-2', 'Barbarian II', 'Palace Software', 1988, 'Combat', 2, ['tosec/Barbarian II (1988)(Palace)(Disk 1 of 2)[cr Replicants].st', 'tosec/Barbarian II (1988)(Palace)(Disk 2 of 2)[cr Replicants].st'], {}),
 ('speedball', 'Speedball', 'Image Works', 1988, 'Sport', 2, ['A_030_speedball.ST'], {'auteur': 'The Bitmap Brothers'}),
 ('bubble-bobble', 'Bubble Bobble', 'Firebird', 1987, 'Plates-formes', 2, ['Bubble Bobble (1987)(Firebird)[cr Lord].st'], {}),
 ('r-type', 'R-Type', 'Electric Dreams', 1988, 'Shoot them up', 1, ['A_046_rtype.ST'], {'touche': '2'}),
 ('crazy-cars-2', 'Crazy Cars II', 'Titus', 1990, 'Course', 1, ['A_049_crazycar2.ST'], {'touche': '1'}),
 ('buggy-boy', 'Buggy Boy', 'Elite', 1988, 'Course', 1, ['A_018_buggyboy.ST'], {'touche': '1', 'touche_type': 'chiffre'}),
 ('ik-plus', 'International Karate +', 'System 3', 1988, 'Combat', 3, ['International Karate + (1988)(Archer Maclean).st'], {'auteur': 'Archer Maclean',
   'correctifs': [{'cherche': '41f900fc0000', 'remplace': '41f900030000',
                   'note': 'IK+ lit la ROM du TOS 1.x en $FC0000 (absente sur STE) : lecture déplacée en RAM, comme IK+ Reforged'}],
   'reforged': [{'type': 'hatari', 'nom': 'IK+ Reforged', 'desc': '3 joueurs, STE', 'dossier': 'reforged/ikplus/IK_PLUS',
                 'auto': r'C:\IK_PLUS.TOS', 'machine': 'ste', 'options': ['--memsize', '1024', '--joy0', 'real']}]}),
 ('captain-blood', 'Captain Blood', 'ERE Informatique', 1988, 'Aventure', 1, ['A_077_capitainblood.ST'], {'touche': '1', 'machine': 'st', 'tos': 'tos/tos104fr.img'}),
 ('populous', 'Populous', 'Bullfrog', 1989, 'Stratégie', 1, ['A_081_populous.ST'], {'touche': '1', 'machine': 'st', 'tos': 'tos/tos206fr.img'}),
 ('dungeon-master', 'Dungeon Master', 'FTL', 1987, 'Jeu de rôle', 1, ['A_097_dungeonmaster.ST'], {'touche': '1'}),
 ('kick-off-2', 'Kick Off 2', 'Anco', 1990, 'Football', 2, ['tosec/Kick Off 2 (1991)(Anco)(fr)[cr Replicants - ST Amigos].st'], {}),
 ('afterburner', 'After Burner', 'Activision', 1988, 'Action', 1, ['A_069_3_afterburner.ST'], {'touche': '1', 'touche_type': 'chiffre'}),
 ('winter-games', 'Winter Games', 'Epyx', 1986, 'Sport', 4, ['Winter Games (1986)(Epyx)[cr CSS].st'], {}),
 ('prince-of-persia', 'Prince of Persia', 'Brøderbund', 1990, 'Action', 1, ['tosec/Prince of Persia (1990)(Broderbund)[cr Empire][m Blue Soft][one disk].st'], {}),
 ('rick-dangerous', 'Rick Dangerous', 'Firebird', 1989, 'Plates-formes', 1, ['Rick Dangerous (1989)(Core Design - Firebird)[cr Medway Boys][t].st'], {'auteur': 'Core Design'}),
 ('rainbow-islands', 'Rainbow Islands', 'Ocean', 1990, 'Plates-formes', 2, ['tosec/Rainbow Islands (1989)(Ocean)[cr Empire][t].st'], {}),
 ('turrican-2', 'Turrican II', 'Rainbow Arts', 1991, 'Action', 1, ['tosec/Turrican II (1991)(Rainbow Arts)[cr Replicants - ST Amigos][t].st'], {}),
 ('chase-hq', 'Chase H.Q.', 'Ocean', 1989, 'Course', 1, ['tosec/Chase HQ (1989)(Ocean).st'], {}),
 ('lotus-turbo', 'Lotus Esprit Turbo Challenge', 'Gremlin', 1990, 'Course', 2, ['A_383_A_lotusturbo.ST'], {'touche': '2', 'tos': 'tos/tos206fr.img'}),
 ('north-and-south', 'North & South', 'Infogrames', 1989, 'Stratégie', 2, ['North & South (1989)(Infogrames)(M5)[cr Corrupt].st'], {}),
 ('defender-of-the-crown', 'Defender of the Crown', 'Cinemaware', 1987, 'Stratégie', 1, ['Defender of the Crown (1987)(Cinemaware)[cr BOSS].st'], {}),
 ('mortevielle', 'Le Manoir de Mortevielle', 'Lankhor', 1987, 'Aventure', 1,
  ["Manoir de Mortevielle, Le (1987)(Lankhor)(Fr)(Disk 1 of 2)[a].st", "Manoir de Mortevielle, Le (1987)(Lankhor)(Fr)(Disk 2 of 2).st"], {}),
 ('super-hang-on', 'Super Hang-On', 'Electric Dreams', 1988, 'Course', 1, ['Super Hang-On (1988)(Sega)[cr Bladerunners].st'], {}),
 ('paperboy', 'Paperboy', 'Elite', 1989, 'Action', 1, ['Paperboy (1989)(Elite).st'], {}),
 ('vroom', 'Vroom', 'Lankhor', 1991, 'Course', 1, ['Vroom (1991)(Lankhor)[cr].st'], {}),
 ('wings-of-death', 'Wings of Death', 'Thalion', 1990, 'Shoot them up', 1,
  ['Wings of Death (1990)(Thalion)(Disk 1 of 2)[cr Empire][t].st', 'Wings of Death (1990)(Thalion)(Disk 2 of 2)[cr Empire][t].st'], {}),
 ('mega-lo-mania', 'Mega lo Mania', 'Image Works', 1991, 'Stratégie', 1,
  ['Mega lo Mania (1991)(Image Works)(Fr)[cr Replicants][t +2](Disk 1 of 2).st', 'Mega lo Mania (1991)(Image Works)(Fr)[cr Replicants][t +2](Disk 2 of 2).st'], {'auteur': 'Sensible Software'}),
 ('chaos-engine', 'The Chaos Engine', 'Renegade', 1993, 'Action', 2,
  ['Chaos Engine, The (1993)(Renegade)(Disk 1 of 2)[cr Cynix][t][a].st', 'Chaos Engine, The (1993)(Renegade)(Disk 2 of 2)[cr Cynix][t][a].st'], {'auteur': 'The Bitmap Brothers'}),
 ('new-zealand-story', 'The New Zealand Story', 'Ocean', 1989, 'Plates-formes', 1, ['tosec/New Zealand Story, The (1989)(Ocean)[cr LSD][one disk].st'], {}),
 ('midwinter', 'Midwinter', 'Rainbird', 1989, 'Stratégie', 1, ['tosec/Midwinter (1990)(Rainbird)[cr Jinx][one disk].st'], {}),
 ('pirates', 'Pirates!', 'MicroProse', 1989, 'Aventure', 1, ['tosec/Pirates (1989)(MicroProse)[cr Replicants].st'], {}),
 ('castle-master', 'Castle Master', 'Incentive', 1990, 'Aventure', 1, ['tosec/Castle Master (1990)(Domark)(M3)[cr Empire].st'], {}),
 ('golden-axe', 'Golden Axe', 'Virgin', 1990, 'Action', 2, ['A_399_goldenaxe.ST'], {'touche': '1'}),
 ('cadaver', 'Cadaver', 'Image Works', 1990, 'Aventure', 1, ['Cadaver (1990)(Bitmap Brothers)[cr Empire][One Disk].st'], {'auteur': 'The Bitmap Brothers'}),
 ('tortues-ninja', 'Teenage Mutant Hero Turtles', 'Image Works', 1990, 'Action', 1, ['Teenage Mutant Hero Turtles (1990)(Image Works)[cr Empire][t].st'], {}),
 ('dynablaster', 'Dynablaster', 'Ubi Soft', 1991, 'Action', 2, ['Dynablaster.st'], {}),
 ('monkey-island', 'The Secret of Monkey Island', 'Lucasfilm Games', 1990, 'Aventure', 1,
  ['tosec/Secret of Monkey Island, The (1991)(LucasFilm Games)(fr)(Disk 1 of 4)[cr Elite].st', 'tosec/Secret of Monkey Island, The (1991)(LucasFilm Games)(fr)(Disk 2 of 4)[cr Elite].st', 'tosec/Secret of Monkey Island, The (1991)(LucasFilm Games)(fr)(Disk 3 of 4)[cr Elite].st', 'tosec/Secret of Monkey Island, The (1991)(LucasFilm Games)(fr)(Disk 4 of 4)[cr Elite].st'], {'auteur': 'Ron Gilbert'}),
 ('loom', 'Loom', 'Lucasfilm Games', 1990, 'Aventure', 1, ['A_463_A_loom.ST', 'A_463_B_loom.ST'], {}),
 ('zak-mckracken', 'Zak McKracken', 'Lucasfilm Games', 1988, 'Aventure', 1, ['zak1.st', 'zak2.st'], {}),
 ('their-finest-hour', 'Their Finest Hour', 'Lucasfilm Games', 1991, 'Simulation', 1,
  ['Their finest Hour - The Battle of Britain (1991)(LucasFilm Games)(Disk 1 of 2).st', 'Their finest Hour - The Battle of Britain (1991)(LucasFilm Games)(Disk 2 of 2).st'], {}),
 ('tintin-lune', 'Tintin sur la Lune', 'Infogrames', 1989, 'Action', 1, ['Tintin on the Moon (19xx)(Infogrames)(M3)[cr V8].st'], {}),
 ('iznogoud', 'Iznogoud', 'Infogrames', 1987, 'Aventure', 1, ['Iznogoud (19xx)(Infogrames)(Fr)[cr].st'], {}),
 ('cannon-fodder', 'Cannon Fodder', 'Virgin', 1994, 'Action', 1,
  ['cannon fodder/Cannon Fodder (1994)(Virgin)(Disk 1 of 3)[cr Cynix].st', 'cannon fodder/Cannon Fodder (1994)(Virgin)(Disk 2 of 3)[cr Cynix].st',
   'cannon fodder/Cannon Fodder (1994)(Virgin)(Disk 3 of 3)[cr Cynix].st'], {'auteur': 'Sensible Software'}),
 ('voyageurs-du-temps', 'Les Voyageurs du Temps', 'Delphine', 1989, 'Aventure', 1,
  ['Voyageur du Temps, Les (19xx)(Delphine)(Fr)(Disk 1 of 3).st', 'Voyageur du Temps, Les (19xx)(Delphine)(Fr)(Disk 2 of 3).st',
   'Voyageur du Temps, Les (19xx)(Delphine)(Fr)(Disk 3 of 3).st'], {}),
 ('silent-service-2', 'Silent Service II', 'MicroProse', 1991, 'Simulation', 1, ['Silent Service II (1991)(Microprose)[cr Dementia].st'], {}),
 ('super-ski-2', 'Super Ski II', 'Microïds', 1990, 'Sport', 4,
  ['Super Ski II (1990)(Microids)[cr Elite](Disk 1 of 2).st', 'Super Ski II (1990)(Microids)[cr Elite](Disk 2 of 2).st'], {}),
 ('carmen-sandiego', 'Carmen Sandiego', 'Microïds', 1991, 'Éducatif', 1,
  ['A La Poursuite de Carmen Sandiego dans le Monde (1991)(Microids)(Fr)[cr Replicants - ST Amigos].st'], {}),
 ('panza-kick-boxing', 'Panza Kick Boxing', 'Loriciel', 1990, 'Combat', 2,
  ['tosec/Panza Kick Boxing (1990)(Loriciel)(FR)(Disk 1 of 2)[cr Empire].st', 'tosec/Panza Kick Boxing (1990)(Loriciel)(FR)(Disk 2 of 2)[cr Empire].st'], {}),
 ('light-corridor', 'The Light Corridor', 'Infogrames', 1990, 'Réflexion', 1, ['A_363_lightcorridor.ST'], {'touche': '2'}),
 ('robocop', 'RoboCop', 'Ocean', 1989, 'Action', 1, ['Robocop (1989)(Ocean)[cr Medway Boys][t Blue Brothers].st'], {}),
 ('hostage', 'Hostage', 'Infogrames', 1988, 'Action', 1, ['A_095_hostage.ST'], {'touche': '2', 'touche_type': 'chiffre'}),
 ('mouse-trap', 'Mouse Trap', 'Microdeal', 1987, 'Plates-formes', 1, ['A_016_mousetrap.ST'], {'touche': '4'}),
 ('super-sprint', 'Super Sprint', 'Electric Dreams', 1988, 'Course', 2, ['ssprint.st'], {}),
 ('lemmings', 'Lemmings', 'Psygnosis', 1991, 'Réflexion', 1, ['tosec/Lemmings (1990)(Psygnosis)[cr BBC].st'], {'auteur': 'DMA Design'}),
 ('speedball-2', 'Speedball 2', 'Image Works', 1990, 'Sport', 2, ['tosec/Speedball 2 - Brutal Deluxe (1990)(Image Works)[cr Replicants - ST Amigos].st'], {'auteur': 'The Bitmap Brothers'}),
 ('another-world', 'Another World', 'Delphine', 1991, 'Action', 1, ['tosec/Another World (1991)(Delphine)(FR)(Disk 1 of 2)[cr Replicants].st', 'tosec/Another World (1991)(Delphine)(FR)(Disk 2 of 2)[cr Replicants].st'], {'auteur': 'Éric Chahi'}),
 ('arkanoid-2', 'Arkanoid II', 'Imagine', 1988, 'Casse-briques', 2, ['tosec/Arkanoid II - Revenge of Doh (1988)(Imagine)[cr Replicants].st'], {}),
 ('strider', 'Strider', 'U.S. Gold', 1989, 'Action', 1, ['tosec/Strider (1989)(U.S. Gold)[cr Replicants][t].st'], {}),
 ('chambers-of-shaolin', 'Chambers of Shaolin', 'Grandslam', 1989, 'Combat', 1, ['Chambers of Shaolin (1989)(Grandslam).st'], {}),
 # --- jeux cultes ajoutés depuis la collection TOSEC (13/10/2026), testés dans Hatari ---
 ('xenon-2', 'Xenon 2 Megablast', 'Image Works', 1989, 'Shoot them up', 1, ['tosec/Xenon 2 - Megablast (1989)(Image Works)[cr Bitcrack Brothers][one disk].st'], {'auteur': 'The Bitmap Brothers'}),
 ('gods', 'Gods', 'Renegade', 1991, 'Plates-formes', 1, ['tosec/Gods (1991)(Renegade)(Disk 1 of 2)[cr Elite][t +3].st', 'tosec/Gods (1991)(Renegade)(Disk 2 of 2)[cr Elite][t +3].st'], {'auteur': 'The Bitmap Brothers'}),
 ('magic-pockets', 'Magic Pockets', 'Renegade', 1991, 'Plates-formes', 1, ['tosec/Magic Pocket (1991)(Renegade)[cr Elite][t].st'], {'auteur': 'The Bitmap Brothers'}),
 ('shadow-of-the-beast', 'Shadow of the Beast', 'Psygnosis', 1989, 'Action', 1, ['tosec/Shadow of the Beast (1990)(Psygnosis)(Disk 1 of 2)[cr Replicants][t].st', 'tosec/Shadow of the Beast (1990)(Psygnosis)(Disk 2 of 2)[cr Replicants][t].st'], {'auteur': 'Reflections'}),
 ('blood-money', 'Blood Money', 'Psygnosis', 1989, 'Shoot them up', 2, ['tosec/Blood Money (1989)(Psygnosis)[cr Delight].st'], {'auteur': 'DMA Design'}),
 ('menace', 'Menace', 'Psygnosis', 1988, 'Shoot them up', 1, ['tosec/Menace (1988)(Psyclapse)[cr Delight][one disk].st'], {'auteur': 'DMA Design'}),
 ('goldrunner', 'Goldrunner', 'Microdeal', 1987, 'Shoot them up', 1, ['tosec/Goldrunner (1987)(Microdeal)[cr 42-Crew].st'], {'auteur': 'Steve Bak'}),
 ('starglider-2', 'Starglider 2', 'Rainbird', 1988, 'Action', 1, ['tosec/Starglider 2 (1988)(Rainbird)[cr Union].st'], {'auteur': 'Argonaut (Jez San)'}),
 ('carrier-command', 'Carrier Command', 'Rainbird', 1988, 'Stratégie', 1, ['tosec/Carrier Command (1988)(Rainbird)(M3)[cr Bladerunners].st'], {'auteur': 'Realtime Games'}),
 ('oids', 'Oids', 'FTL', 1987, 'Action', 1, ['tosec/Oids (1987)(FTL)[cr Was -Not Was-].st'], {'auteur': 'Dan Hewitt'}),
 ('llamatron', 'Llamatron', 'Llamasoft', 1991, 'Shoot them up', 2, ['tosec/Llamatron v1.0 (1991-03-20)(Yak)(SW)[1MB].st'], {'auteur': 'Jeff Minter'}),
 ('gauntlet-2', 'Gauntlet II', 'U.S. Gold', 1989, 'Action', 2, ['tosec/Gauntlet II (1986)(Atari Corp.)[cr Medway Boys][t][one disk].st'], {}),
 ('bomb-jack', 'Bomb Jack', 'Elite', 1987, 'Plates-formes', 1, ['tosec/Bombjack (1988)(Elite).st'], {}),
 ('out-run', 'Out Run', 'U.S. Gold', 1988, 'Course', 1, ['tosec/Out Run (1987)(U.S. Gold)[cr Bladerunners].st'], {}),
 ('super-cars', 'Super Cars', 'Gremlin', 1990, 'Course', 1, ['tosec/Super Cars (1990)(Gremlin)[cr Replicants].st'], {'auteur': 'Magnetic Fields'}),
 ('pang', 'Pang', 'Ocean', 1990, 'Action', 2, ['tosec/Pang (1990)(Ocean)[cr Empire][t].st'], {}),
 ('toki', 'Toki', 'Ocean', 1991, 'Plates-formes', 1, ['tosec/Toki (1991)(Ocean)[cr Replicants - ST Amigos].st'], {}),
 ('rodland', 'Rodland', 'Storm', 1991, 'Plates-formes', 2, ['tosec/Rodland (1991)(Sales Curve)[cr Elite][t].st'], {}),
 ('ghouls-n-ghosts', "Ghouls'n Ghosts", 'U.S. Gold', 1989, 'Action', 1, ["tosec/Ghouls 'n' Ghosts (1988)(U.S. Gold)[cr Empire][one disk].st"], {}),
 ('silkworm', 'Silkworm', 'Virgin', 1989, 'Shoot them up', 2, ['tosec/Silk Worm (1988)(Tecmo)[cr Replicants].st'], {}),
 ('enchanted-land', 'Enchanted Land', 'Thalion', 1990, 'Plates-formes', 1, ['tosec/Enchanted Land (1990)(Thalion)[cr Empire].st'], {}),
 ('lethal-xcess', 'Lethal Xcess', 'Eclipse', 1991, 'Shoot them up', 2, ['tosec/Lethal Xcess - Wings of Death II (1991)(Eclipse)(Disk 1 of 2)(Disk A)[cr Elite][t].st', 'tosec/Lethal Xcess - Wings of Death II (1991)(Eclipse)(Disk 2 of 2)(Disk B)[cr Elite][t].st'], {}),
 ('turrican', 'Turrican', 'Rainbow Arts', 1990, 'Action', 1, ['tosec/Turrican (1990)(Rainbow Arts)[cr Empire][t].st'], {'auteur': 'Manfred Trenz'}),
 ('rick-dangerous-2', 'Rick Dangerous 2', 'Micro Style', 1990, 'Plates-formes', 1, ['tosec/Rick Dangerous II (1989)(Core Design)[cr Replicants].st'], {'auteur': 'Core Design'}),
 ('nebulus', 'Nebulus', 'Hewson', 1988, 'Plates-formes', 1, ['tosec/Nebulus (1988)(Hewson)[cr Atari Legend].st'], {'auteur': 'John Phillips'}),
 ('cybernoid', 'Cybernoid', 'Hewson', 1988, 'Shoot them up', 1, ['tosec/Cybernoid (1988)(Hewson).st'], {'auteur': 'Raffaele Cecco'}),
 ('pacmania', 'Pac-Mania', 'Grandslam', 1988, 'Arcade', 1, ['tosec/Pacmania (1988)(Grandslam).st'], {}),
 ('arkanoid', 'Arkanoid', 'Imagine', 1987, 'Casse-briques', 1, ['tosec/Arkanoid (1987)(Imagine)[cr Anit-Satanist].st'], {}),
 ('space-harrier', 'Space Harrier', 'Elite', 1987, 'Shoot them up', 1, ['tosec/Space Harrier (1988)(Elite)[cr MCA][one disk].st'], {}),
 ('double-dragon', 'Double Dragon', 'Melbourne House', 1988, 'Combat', 2, ['tosec/Double Dragon (1988)(Tradewest)[cr 42-Crew].st'], {}),
 ('james-pond', 'James Pond', 'Millennium', 1990, 'Plates-formes', 1, ['tosec/James Pond - Underwater Agent (1990)(Vectordean)[cr Replicants].st'], {}),
 ('robocod', 'James Pond 2 : Robocod', 'Millennium', 1991, 'Plates-formes', 1, ['tosec/James Pond II - Codename Robocod (1992)(Vectordean)[cr Vmax].st'], {}),
 ('wizball', 'Wizball', 'Ocean', 1988, 'Shoot them up', 1, ['tosec/Wizball (1987)(Ocean)[cr Beng].st'], {'auteur': 'Sensible Software'}),
 ('fantasy-world-dizzy', 'Fantasy World Dizzy', 'Codemasters', 1989, 'Plates-formes', 1, ['tosec/Fantasy World Dizzy (1991)(Codemasters)(GB).stx'], {'auteur': 'The Oliver Twins'}),
 ('bubble-ghost', 'Bubble Ghost', 'ERE Informatique', 1987, 'Réflexion', 1, ['tosec/Bubble Ghost (1987)(ERE).st'], {'auteur': 'Christophe Andréani'}),
 ('skweek', 'Skweek', 'Loriciel', 1989, 'Arcade', 1, ['tosec/Skweek (1989)(Loriciel)[cr Delight][f French Tos].st'], {}),
 ('tetris', 'Tetris', 'Mirrorsoft', 1988, 'Réflexion', 1, ['tosec/Tetris (1987)(MirrorSoft)[cr Lord].st'], {}),
 ('klax', 'Klax', 'Domark', 1990, 'Réflexion', 2, ['tosec/Klax (1990)(Atari).stx'], {}),
 ('puzznic', 'Puzznic', 'Ocean', 1990, 'Réflexion', 1, ['tosec/Puzznic (1990)(Ocean).stx'], {}),
 ('bombuzal', 'Bombuzal', 'Image Works', 1988, 'Réflexion', 1, ['tosec/Bombuzal (1988)(Image Works)[cr Bladerunners].st'], {}),
 ('ballistix', 'Ballistix', 'Psygnosis', 1989, 'Sport', 2, ['tosec/Ballistix (1989)(Psyclapse)[cr Replicants].st'], {}),
 ('bolo', 'Bolo', 'Application Systems', 1987, 'Casse-briques', 1, ['tosec/Bolo (1987)(Application Systems Heidelberg)(en-de).st'], {}),
 ('oxyd', 'Oxyd', 'Dongleware', 1990, 'Réflexion', 1, ['tosec/Oxyd (1990)(Digital Art).st'], {}),
 ('archipelagos', 'Archipelagos', 'Logotron', 1989, 'Réflexion', 1, ['tosec/Archipelagos (1989)(Logotron)[cr Medway Boys][use pasti].st'], {}),
 ('sundog', 'Sundog : Frozen Legacy', 'FTL', 1985, 'Aventure', 1, ['tosec/Sundog - Frozen Legacy v3.0 (1985)(FTL).st'], {}),
 ('crafton-xunk', 'Crafton & Xunk', 'ERE Informatique', 1986, 'Aventure', 1, ["tosec/Crafton et Xunk - L'ange de Cristal (1988)(ERE)(M4).stx"], {}),
 ('sapiens', 'Sapiens', 'Loriciel', 1986, 'Aventure', 1, ['tosec/Sapiens (1987)(Loriciel).st'], {}),
 ('ikari-warriors', 'Ikari Warriors', 'Elite', 1988, 'Action', 2, ['tosec/Ikari Warriors (1987)(Elite).st'], {}),
 ('virus', 'Virus', 'Firebird', 1988, 'Shoot them up', 1, ['tosec/Virus (1988)(Braben, D.J.)[cr BCS].st'], {'auteur': 'David Braben'}),
 ('frontier', 'Frontier : Elite II', 'Konami', 1993, 'Simulation', 1, ['tosec/Frontier - Elite II (1993)(Gametek - Konami)(fr).st'], {'auteur': 'David Braben'}),
 ('mercenary', 'Mercenary', 'Novagen', 1987, 'Aventure', 1, ['tosec/Mercenary - Escape From Targ (1986)(Novasoft)[cr BOSS].st'], {'auteur': 'Paul Woakes'}),
 ('damocles', 'Damocles', 'Novagen', 1990, 'Aventure', 1, ['tosec/Mercenary 2 - Damocles (1990)(Novagen)[cr Medway Boys].st'], {'auteur': 'Paul Woakes'}),
 ('hunter', 'Hunter', 'Activision', 1991, 'Action', 1, ['tosec/Hunter (1991)(Activision)[cr Paper Bag].st'], {}),
 ('f29-retaliator', 'F-29 Retaliator', 'Ocean', 1989, 'Simulation', 1, ['tosec/F-29 Retaliator (1989)(Ocean)[cr Empire].st'], {'auteur': 'Digital Image Design'}),
 ('falcon', 'Falcon', 'Mirrorsoft', 1988, 'Simulation', 1, ['tosec/Falcon (1989)(Spectrum Holobyte)(Disk 1 of 2)[!].stx', 'tosec/Falcon (1989)(Spectrum Holobyte)(Disk 2 of 2)[!].stx'], {'auteur': 'Spectrum HoloByte'}),
 ('gunship', 'Gunship', 'MicroProse', 1987, 'Simulation', 1, ['tosec/Gunship (1987)(MicroProse)[cr Angels of Darkness][one disk].st'], {}),
 ('hard-drivin', "Hard Drivin'", 'Domark', 1989, 'Course', 1, ["tosec/Hard Drivin' (1989)(Domark)[cr Replicants].st"], {}),
 ('super-monaco-gp', 'Super Monaco GP', 'U.S. Gold', 1991, 'Course', 1, ['tosec/Super Monaco GP (1991)(U.S. Gold)[cr Hotline - MCA - Nokturnal].st'], {}),
 ('f1-grand-prix', 'Formula One Grand Prix', 'MicroProse', 1992, 'Course', 1, ['tosec/Formula One Grand Prix (1991)(MicroProse)(M3)(Disk 1 of 2)[cr Bastards].st', 'tosec/Formula One Grand Prix (1991)(MicroProse)(M3)(Disk 2 of 2)[cr Bastards].st'], {'auteur': 'Geoff Crammond'}),
 ('indy-500', 'Indianapolis 500', 'Electronic Arts', 1990, 'Course', 1, ['tosec/Indianapolis 500 (1989)(Electronic Arts)[cr Bladerunners].st'], {'auteur': 'Papyrus'}),
 ('powerdrome', 'Powerdrome', 'Electronic Arts', 1988, 'Course', 1, ['tosec/Powerdrome (1989)(Electronic Arts)[cr STCS].st'], {'auteur': 'Michael Powell'}),
 ('turbo-cup', 'Turbo Cup', 'Loriciel', 1988, 'Course', 1, ['tosec/Turbo Cup (1988)(Loriciel)[cr Bladerunners][m Blue Soft][one disk].st'], {}),
 ('sensible-soccer', 'Sensible Soccer', 'Renegade', 1992, 'Football', 2, ['tosec/Sensible Soccer (1992)(Renegade)(M4)[cr Vmax][one disk].st'], {'auteur': 'Sensible Software'}),
 ('microprose-soccer', 'MicroProse Soccer', 'MicroProse', 1989, 'Football', 2, ['tosec/Microprose Soccer (1989)(MicroProse).st'], {'auteur': 'Sensible Software'}),
 ('leaderboard', 'Leader Board', 'Access', 1986, 'Sport', 4, ['tosec/Leader Board Pro Golf Simulator (1986)(Access)[cr BOSS].st'], {}),
 ('california-games', 'California Games', 'Epyx', 1988, 'Sport', 4, ['tosec/California Games (1989)(Epyx - U.S. Gold)[cr Bad Brew Crew].st'], {}),
 ('purple-saturn-day', 'Purple Saturn Day', 'Exxos', 1989, 'Sport', 1, ['tosec/Purple Saturn Day (1989)(Exxos)[cr Bladerunners].st'], {}),
 ('sim-city', 'SimCity', 'Infogrames', 1989, 'Simulation', 1, ['tosec/Sim City (1989)(Maxis)[cr Empire].st'], {'auteur': 'Will Wright'}),
 ('civilization', 'Civilization', 'MicroProse', 1992, 'Stratégie', 1, ['tosec/Civilization (1993)(MicroProse)(Disk 1 of 4)(Disk 0)[cr ICS].st', 'tosec/Civilization (1993)(MicroProse)(Disk 2 of 4)(Disk A)[cr ICS].st', 'tosec/Civilization (1993)(MicroProse)(Disk 3 of 4)(Disk B)[cr ICS].st', 'tosec/Civilization (1993)(MicroProse)(Disk 4 of 4)(Disk C)[cr ICS].st'], {'auteur': 'Sid Meier'}),
 ('railroad-tycoon', 'Railroad Tycoon', 'MicroProse', 1991, 'Stratégie', 1, ['tosec/Railroad Tycoon v755.01 (1991)(MicroProse)[cr Medway Boys][m MCA][one disk].st'], {'auteur': 'Sid Meier'}),
 ('populous-2', 'Populous II', 'Electronic Arts', 1991, 'Stratégie', 1, ['tosec/Populous II (1991)(Bullfrog)[cr ICS][512 kB].st'], {'auteur': 'Bullfrog'}),
 ('powermonger', 'Powermonger', 'Electronic Arts', 1990, 'Stratégie', 1, ['tosec/Powermonger (1990)(Bullfrog)[cr Empire].st'], {'auteur': 'Bullfrog'}),
 ('lemmings-2', 'Oh No! More Lemmings', 'Psygnosis', 1991, 'Réflexion', 1, ['tosec/Lemmings 2 - Oh no, More Lemmings (1991)(Psygnosis)[cr Elite][t].st'], {'auteur': 'DMA Design'}),
 ('battle-chess', 'Battle Chess', 'Interplay', 1988, 'Réflexion', 2, ['tosec/Battle Chess (1989)(Interplay)[cr MCA].st'], {}),
 ('bloodwych', 'Bloodwych', 'Image Works', 1989, 'Jeu de rôle', 2, ['tosec/Bloodwych (1989)(MirrorSoft)[cr Equinox].st'], {}),
 ('chaos-strikes-back', 'Chaos Strikes Back', 'FTL', 1989, 'Jeu de rôle', 1, ['tosec/Chaos Strikes Back v2.1 (1989)(FTL)(Disk 1 of 2)(Game Disk)[!].stx', 'tosec/Chaos Strikes Back v2.1 (1989)(FTL)(Disk 2 of 2)(Game Disk)[!].stx'], {}),
 ('captive', 'Captive', 'Mindscape', 1990, 'Jeu de rôle', 1, ['tosec/Captive (1990)(Mindscape)(Disk 1 of 2).stx', 'tosec/Captive (1990)(Mindscape)(Disk 2 of 2).stx'], {'auteur': 'Tony Crowther'}),
 ('ishar', 'Ishar', 'Silmarils', 1992, 'Jeu de rôle', 1, ['tosec/Ishar - Legend of the Fortress (1992)(Silmarils)(FR)(Disk 1 of 2)[cr Replicants][m Fuzion].st', 'tosec/Ishar - Legend of the Fortress (1992)(Silmarils)(FR)(Disk 2 of 2)[cr Replicants][m Fuzion].st'], {}),
 ('drakkhen', 'Drakkhen', 'Infogrames', 1989, 'Jeu de rôle', 1, ['tosec/Drakkhen (1989)(Infogrames)(Disk 1 of 4).st', 'tosec/Drakkhen (1989)(Infogrames)(Disk 2 of 4).st', 'tosec/Drakkhen (1989)(Infogrames)(Disk 3 of 4).st', 'tosec/Drakkhen (1989)(Infogrames)(Disk 4 of 4).st'], {}),
 ('dragonflight', 'Dragonflight', 'Thalion', 1990, 'Jeu de rôle', 1, ['tosec/Dragonflight (1990)(Thalion)(Disk 1 of 2)(Disk A)[cr Atari Legend][t].st', 'tosec/Dragonflight (1990)(Thalion)(Disk 2 of 2)(Disk B - C)[cr Atari Legend][t].st'], {}),
 ('ultima-iv', 'Ultima IV', 'Origin', 1987, 'Jeu de rôle', 1, ['tosec/Ultima IV - Quest of the Avatar (1988)(Origin)[cr TEX].st'], {'auteur': 'Richard Garriott'}),
 ('maniac-mansion', 'Maniac Mansion', 'Lucasfilm Games', 1988, 'Aventure', 1, ['tosec/Maniac Mansion (1988)(LucasFilm Games)(fr)[one disk].st'], {'auteur': 'Ron Gilbert'}),
 ('indy-croisade', 'Indiana Jones et la Dernière Croisade', 'Lucasfilm Games', 1989, 'Aventure', 1, ['tosec/Indiana Jones et la Derniere Croisade (1989)(LucasFilm Games)(fr)(Disk 1 of 3)[3 disks version].st', 'tosec/Indiana Jones et la Derniere Croisade (1989)(LucasFilm Games)(fr)(Disk 2 of 3)[3 disks version].st', 'tosec/Indiana Jones et la Derniere Croisade (1989)(LucasFilm Games)(fr)(Disk 3 of 3)[3 disks version].st'], {}),
 ('operation-stealth', 'Operation Stealth', 'Delphine', 1990, 'Aventure', 1, ['tosec/Operation Stealth (1990)(U.S. Gold)(fr)(Disk 1 of 3)[cr Replicants].st', 'tosec/Operation Stealth (1990)(U.S. Gold)(fr)(Disk 2 of 3)[cr Replicants].st', 'tosec/Operation Stealth (1990)(U.S. Gold)(fr)(Disk 3 of 3)[cr Replicants].st'], {}),
 ('cruise-for-a-corpse', 'Croisière pour un cadavre', 'Delphine', 1991, 'Aventure', 1, ['tosec/Cruise for a Corpse (1991)(U.S. Gold)(Disk 1 of 5)[cr Elite].st', 'tosec/Cruise for a Corpse (1991)(U.S. Gold)(Disk 2 of 5)[cr Elite].st', 'tosec/Cruise for a Corpse (1991)(U.S. Gold)(Disk 3 of 5)[cr Elite].st', 'tosec/Cruise for a Corpse (1991)(U.S. Gold)(Disk 4 of 5)[cr Elite].st', 'tosec/Cruise for a Corpse (1991)(U.S. Gold)(Disk 5 of 5)[cr Elite].st'], {}),
 ('gobliiins', 'Gobliiins', 'Coktel Vision', 1991, 'Aventure', 1, ['tosec/Gobliiins (1991)(CVS)(M5)(Disk 1 of 3)[cr Elite].st', 'tosec/Gobliiins (1991)(CVS)(M5)(Disk 2 of 3)[cr Elite].st', 'tosec/Gobliiins (1991)(CVS)(M5)(Disk 3 of 3)[cr Elite].st'], {}),
 ('passagers-du-vent', 'Les Passagers du Vent', 'Infogrames', 1986, 'Aventure', 1, ['tosec/Passagers du Vent I, Les (1987)(Infogrames)(FR)[cr][m Atariforce].st'], {}),
 ('kult', 'Kult', 'Exxos', 1989, 'Aventure', 1, ['tosec/Kult (1989)(ERE)(M3)[cr Replicants][one disk].st'], {}),
 ('bat', 'B.A.T.', 'Ubi Soft', 1990, 'Aventure', 1, ['tosec/B.A.T. (1990)(UBI Soft)(FR)(Disk 1 of 3)[cr Replicants].st', 'tosec/B.A.T. (1990)(UBI Soft)(FR)(Disk 2 of 3)[cr Replicants].st', 'tosec/B.A.T. (1990)(UBI Soft)(FR)(Disk 3 of 3)[cr Replicants].st'], {}),
 ('maupiti-island', 'Maupiti Island', 'Lankhor', 1990, 'Aventure', 1, ['tosec/Maupiti Island (1990)(Lankhor)(Disk 1 of 2)[cr Medway Boys].st', 'tosec/Maupiti Island (1990)(Lankhor)(Disk 2 of 2)[cr Medway Boys].st'], {}),
 ('iron-lord', 'Iron Lord', 'Ubi Soft', 1989, 'Aventure', 1, ['tosec/Iron Lord (1989)(UBI Soft)(FR)(Disk 1 of 3)[cr Replicants].st', 'tosec/Iron Lord (1989)(UBI Soft)(FR)(Disk 2 of 3)[cr Replicants].st', 'tosec/Iron Lord (1989)(UBI Soft)(FR)(Disk 3 of 3)[cr Replicants].st'], {}),
 ('prehistorik', 'Prehistorik', 'Titus', 1991, 'Plates-formes', 1, ['tosec/Prehistorik (1991)(Titus)[cr Replicants - ST Amigos].st'], {}),
 ('titus-the-fox', 'Titus the Fox', 'Titus', 1991, 'Plates-formes', 1, ['tosec/Titus the Fox - To Marrakech and Back (1992)(Titus)[cr Replicants].st'], {}),
 ('blues-brothers', 'The Blues Brothers', 'Titus', 1991, 'Plates-formes', 2, ['tosec/Blues Brothers, The (1991)(Titus)[cr Empire][t].st'], {}),
 ('parasol-stars', 'Parasol Stars', 'Ocean', 1992, 'Plates-formes', 2, ['tosec/Parasol Stars (1992)(Ocean)[cr Cynix][t +11 Cynix].st'], {}),
 ('wonder-boy', 'Wonder Boy in Monster Land', 'Activision', 1989, 'Plates-formes', 1, ['tosec/Wonderboy in Monsterland (1987)(Activision)[cr Vapour][one disk].st'], {}),
 ('switchblade', 'Switchblade', 'Gremlin', 1989, 'Plates-formes', 1, ['tosec/Switchblade (1989)(Core Design)[cr MCA].st'], {}),
 ('final-fight', 'Final Fight', 'U.S. Gold', 1991, 'Combat', 2, ['tosec/Final Fight (1991)(Capcom)(Disk 1 of 2)[cr Elite][t].st', 'tosec/Final Fight (1991)(Capcom)(Disk 2 of 2)[cr Elite][t].st'], {}),
 ('shinobi', 'Shinobi', 'Virgin', 1989, 'Action', 1, ['tosec/Shinobi (1989)(Sales Curve)[cr Rebels][t].st'], {}),
]

# réglages machine des jeux TOSEC (testés) : STF + TOS 1.04 pour les plus anciens
_ST = {'machine': 'st', 'tos': 'tos/tos104fr.img'}
REGLAGES = {
 'klax': dict(_ST, options=['--fastfdc', 'no']), 'puzznic': dict(_ST, options=['--fastfdc', 'no']),
 'enchanted-land': _ST, 'prehistorik': _ST,
 'f29-retaliator': {'options': ['--memsize', '1024', '--fastfdc', 'no']},
 'dragonflight': {'options': ['--memsize', '1024']}, 'civilization': {'options': ['--memsize', '1024']},
 'bolo': _ST, 'captain-blood': _ST,
}
for _j in J:
    _j[7].update(REGLAGES.get(_j[0], {}))


AUTO = {'monkey-island': r'A:\MONKEY.PRG', 'mortevielle': r'A:\MORT.PRG', 'their-finest-hour': r'A:\BOB.PRG',
        'voyageurs-du-temps': r'A:\DELPHINE.PRG', 'zak-mckracken': r'A:\ZAK.PRG', 'robocop': r'A:\ROBOCOP\ROBOBOY.PRG',
        'super-hang-on': r'A:\HANG_ON.PRG', 'defender-of-the-crown': r'A:\ALESTART.TOS', 'ik-plus': r'A:\IK+.PRG',
        # jeux TOSEC qui s'arrêtent sur le bureau GEM
        'menace': r'A:\MENACE.TOS', 'goldrunner': r'A:\GOLDRUN.42C\GOLDRUN.PRG',
        'starglider-2': r'A:\STARGL_2.TOS', 'carrier-command': r'A:\CARRIER.PRG', 'llamatron': r'A:\TRON1MEG.PRG',
        'bomb-jack': r'A:\BJ.PRG', 'out-run': r'A:\OUT_RUN\OUT_RUN.PRG', 'arkanoid': r'A:\ARKANOID.PRG',
        'space-harrier': r'A:\SPACE_H.PRG', 'double-dragon': r'A:\DOUBLE.PRG', 'wizball': r'A:\WIZBALL\WIZBALL.TOS',
        'bubble-ghost': r'A:\GHOST.PRG', 'bolo': r'A:\BOLO.TOS', 'oxyd': r'A:\OXYD.PRG', 'sundog': r'A:\SUNDOG.PRG',
        'virus': r'A:\VIRUS\VIRUS.PRG', 'gunship': r'A:\GUNSHIP.TOS', 'indy-500': r'A:\INDY500.PRG',
        'turbo-cup': r'A:\TURBOCUP.SYS\TURBOCUP.TOS', 'sensible-soccer': r'A:\VMAX.PRG', 'california-games': r'A:\GAMES.PRG',
        'purple-saturn-day': r'A:\PURPLE.PRG', 'sim-city': r'A:\SIMCITY.PRG', 'civilization': r'A:\ICS.PRG',
        'railroad-tycoon': r'A:\GAME.PRG', 'maniac-mansion': r'A:\MANIAC.PRG', 'indy-croisade': r'A:\INDY.PRG',
        'passagers-du-vent': r'A:\VENT1.PRG', 'bat': r'A:\BAT.PRG', 'iron-lord': r'A:\IRONLORD.PRG',
        'speedball-2': r'A:\SPEED\BALL.PRG', 'silent-service-2': r'A:\SSII.PRG', 'carmen-sandiego': r'A:\CARMEN.PRG',
        'new-zealand-story': r'A:\NEWZEAL.TOS', 'loom': r'A:\LOOM.TOS'}

# menus de compil : touche du pavé numérique (ou « c » + chiffre de la rangée du haut)
TOUCHES = {'xenon': '5', 'barbarian': '3', 'speedball': '2', 'populous': '1', 'dungeon-master': '1',
           'captain-blood': '1', 'lotus-turbo': '2', 'buggy-boy': 'c1', 'afterburner': 'c1',
           'crazy-cars-2': '1', 'golden-axe': '1', 'light-corridor': 'c2',
           'r-type': '2', 'hostage': 'c2', 'mouse-trap': '4'}
# écrans d'intro qui attendent Espace (pressée pendant le turbo)
for _i in ['rick-dangerous', 'shufflepuck-cafe', 'cadaver', 'chaos-engine', 'cannon-fodder',
           'tintin-lune', 'tortues-ninja', 'wings-of-death', 'super-ski-2', 'vroom', 'silent-service-2',
           'carmen-sandiego', 'dynablaster', 'iznogoud', 'prince-of-persia']:
    TOUCHES[_i] = 'esp'

# séquences de touches pressées à tour de rôle pendant 5 s au démarrage (prioritaires sur TOUCHES) :
# espace, entree, F1…F10, pave0…pave9, 0…9 (rangée des chiffres), N (« Trainer Y/N ? »)…
SEQ = {'pang': ['N', 'espace'], 'toki': ['espace', 'N'], 'turrican': ['N'], 'dragonflight': ['N'],
       'gobliiins': ['espace', 'pave5', 'entree'], 'maniac-mansion': ['entree'], 'railroad-tycoon': ['pave2'],
       'carrier-command': ['espace', '3'],
       # « Trainer Y/N ? » des cracks : N
       'gods': ['espace', 'N'], 'rainbow-islands': ['espace', 'N'], 'turrican-2': ['espace', 'N'], 'castle-master': ['F', 'espace'], 'midwinter': ['espace'], 'barbarian-2': ['espace'],
       'robocop': ['F1'], 'zak-mckracken': ['entree'], 'monkey-island': ['1', 'entree'], 'loom': ['espace'],
       'mega-lo-mania': ['espace', 'N'], 'chaos-engine': ['espace'], 'cannon-fodder': ['espace'],
       'kick-off-2': ['espace'], 'llamatron': [], 'speedball-2': ['N', 'espace'], 'tortues-ninja': ['espace', 'N'],
       'wings-of-death': ['espace', 'N'], 'blues-brothers': ['espace', 'N'], 'final-fight': ['espace', 'N'],
       'vroom': ['espace', 'N'],
       # jeux Lucasfilm (SCUMM) : Espace met le jeu en pause, rien à presser
       'indy-croisade': []}
# intros de crack qui arrivent tard : touches pressées plus longtemps (secondes)
DUREE = {'lemmings': 15, 'another-world': 15, 'llamatron': 15, 'rick-dangerous-2': 15, 'battle-chess': 15,
         'ishar': 15, 'cruise-for-a-corpse': 15, 'blues-brothers': 15, 'final-fight': 15, 'vroom': 15,
         'gods': 12, 'rainbow-islands': 30, 'speedball-2': 12, 'tortues-ninja': 12, 'wings-of-death': 12, 'zak-mckracken': 15,
         'monkey-island': 25, 'loom': 12, 'mega-lo-mania': 15, 'chaos-engine': 15, 'cannon-fodder': 15,
         'robocop': 10, 'midwinter': 15, 'kick-off-2': 15,
         'castle-master': 25, 'barbarian-2': 15}
# secondes à attendre avant de presser les touches (l'écran qui les attend arrive tard)
DELAI = {'monkey-island': 40, 'barbarian-2': 8, 'zak-mckracken': 3, 'chaos-engine': 18, 'cannon-fodder': 18, 'midwinter': 15,
         'kick-off-2': 8}
# les autres jeux TOSEC : intro de crack ou « press space » -> Espace
for _j in J:
    if _j[6][0].startswith('tosec/') and _j[0] not in SEQ and _j[0] not in TOUCHES:
        SEQ[_j[0]] = ['espace']

# jeux qui se jouent à la souris (le port 0 reste à la souris)
SOURIS = {'shufflepuck-cafe', 'populous', 'mortevielle', 'dungeon-master', 'captain-blood', 'lemmings', 'arkanoid-2',
          'north-and-south', 'defender-of-the-crown', 'vroom', 'mega-lo-mania', 'midwinter', 'castle-master', 'cadaver',
          'monkey-island', 'loom', 'zak-mckracken', 'iznogoud', 'cannon-fodder', 'voyageurs-du-temps',
          'silent-service-2', 'carmen-sandiego', 'light-corridor',
          'carrier-command', 'arkanoid', 'bubble-ghost', 'bolo', 'oxyd', 'archipelagos', 'sundog', 'virus', 'frontier',
          'sim-city', 'civilization', 'railroad-tycoon', 'populous-2', 'powermonger', 'lemmings-2',
          'battle-chess', 'bloodwych', 'chaos-strikes-back', 'captive', 'ishar', 'drakkhen', 'dragonflight',
          'maniac-mansion', 'indy-croisade', 'operation-stealth', 'cruise-for-a-corpse', 'gobliiins',
          'passagers-du-vent', 'kult', 'bat', 'maupiti-island', 'falcon', 'crafton-xunk'}

# rappel des commandes du jeu, affiché sur la fiche
AIDE = {
 'stunt-car-racer': 'Haut : accélérer · bas : freiner · tir : boost',
 'shufflepuck-cafe': 'Souris : la raquette · bouton : frappe forte · Espace : menu',
 'prince-of-persia': 'Tir maintenu : s\'agripper, dégainer, frapper',
 'rick-dangerous': 'Tir + gauche/droite : tirer · tir + bas : bombe · tir + haut : bâton',
 'ik-plus': 'Coups : directions avec ou sans tir (8 + 8 mouvements)',
 'barbarian': 'F1 : partie à 1 joueur · F2 : partie à 2 joueurs · Tir + direction : coups d\'épée',
 'bubble-bobble': '1 ou 2 : commencer à 1 ou 2 joueurs · Tir : bulle · Haut : saut',
 'populous': 'Souris : bouton gauche aplanit, droit creuse',
 'mortevielle': 'Souris : menus en haut de l\'écran',
 'lemmings': 'Souris : choisir une capacité, cliquer sur un lemming',
 'arkanoid-2': 'Souris : la raquette · bouton : tirer',
 'speedball-2': 'Tir : passe ou tacle · tir maintenu : tir puissant',
 'speedball': 'Tir : passe ou tacle · tir maintenu : tir puissant',
 'another-world': 'Tir : action, tir maintenu : charger le pistolet',
 'buggy-boy': 'Haut : accélérer · tir : changer de vitesse',
 'xenon': 'Tir : tirer (et se transformer entre deux vagues)',
 'super-sprint': 'Tir : accélérer · jusqu\'à 2 joueurs',
 'mouse-trap': 'Tir : sauter',
 'lotus-turbo': 'Tir : accélérer · Bas : freiner · jusqu\'à 2 joueurs en écran partagé',
 'blood-money': 'F1 : 1 joueur · F2 : 2 joueurs · Tir : tirer',
 'bloodwych': 'F1 : partie à 1 joueur · F2 : partie à 2 joueurs (écran partagé)',
 'microprose-soccer': 'F1 : football en plein air · F2 : football en salle',
 'oxyd': 'Souris : guider la bille · F1 : commencer',
 'bomb-jack': 'F1 ou tir : commencer · Tir : sauter',
 'double-dragon': '1 ou 2 : commencer à 1 ou 2 joueurs',
 'pang': 'Tir : harpon · jusqu\'à 2 joueurs',
 'titus-the-fox': '1 : commencer · 2 : mot de passe',
 'xenon-2': 'Tir : tirer · tir maintenu : tir continu',
 'operation-stealth': 'Protection : cliquer la pièce qui clignote, puis la bonne couleur',
 'cruise-for-a-corpse': 'Protection : régler le code avec les flèches jaunes, puis OK',
 'kult': 'Souris : cliquer le drapeau de la langue',
 'crafton-xunk': 'Souris : cliquer le drapeau de la langue',
 'f1-grand-prix': 'Souris : choisir la langue, puis OK',
 'leaderboard': 'Clavier : nombre de joueurs puis les noms, Entrée',
 'powermonger': 'Clavier : votre nom, puis Entrée',
 'frontier': 'Souris : commandes du vaisseau en bas de l\'écran',
 'super-cars': 'Clavier : votre nom, puis Entrée · Tir : accélérer',
 'gauntlet-2': 'Tir : choisir son héros · jusqu\'à 2 joueurs',
 'starglider-2': 'Mot de passe demandé : SPREAD',
 'gunship': 'Protection : choisir le véhicule montré (souris ou joystick, puis tir)',
 'arkanoid': 'Souris : la raquette · clic : commencer',
 'lethal-xcess': 'Tir : commencer · jusqu\'à 2 joueurs',
 'nebulus': 'F1 / F2 : 1 ou 2 joueurs · Tir : commencer',
 'bolo': 'Souris : la raquette',
 'carrier-command': 'Souris : tout se commande depuis le pont du porte-avions',
 'llamatron': 'Tir : passer l\'écran shareware · 2 joueurs : manette 2',
 'monkey-island': 'Question du pirate : n\'importe quel nombre puis Entrée (version crackée)',
}

ACTIFS_INITIAUX = {'stunt-car-racer', 'shufflepuck-cafe', 'lemmings', 'prince-of-persia', 'lotus-turbo', 'speedball-2',
          'populous', 'rick-dangerous', 'ik-plus', 'barbarian', 'mortevielle', 'hostage', 'mouse-trap',
          'another-world', 'arkanoid-2', 'bubble-bobble', 'strider', 'buggy-boy'}
# depuis l'arrivée de TOSEC (13/10/2026) : tout le catalogue est dans le bac,
# sauf les jeux qui ne démarrent pas (à recocher avec F10)
INACTIFS = set()
ACTIFS = {j[0] for j in J} - INACTIFS


def h(s, n):
    return int(hashlib.md5(s.encode()).hexdigest(), 16) % n


jeux = []
for (i, t, e, a, g, p, d, x) in J:
    v0 = {'type': 'hatari', 'nom': 'Original ST', 'desc': 'Hatari' + (', %d disquettes' % len(d) if len(d) > 1 else ''),
          'disques': ['disques/' + f for f in d]}
    for k in ('machine', 'tos', 'correctifs', 'options'):
        if k in x:
            v0[k] = x[k]
    if i in AUTO:
        v0['auto'] = AUTO[i]
    v = [v0]
    v += x.get('reforged', [])
    j = {'id': i, 'titre': t, 'editeur': e, 'annee': a, 'genre': g, 'joueurs': p,
         'disquette': {'corps': CORPS[h(i + 'c', len(CORPS))], 'bande': BANDES[h(i, len(BANDES))],
                       'police': POLICES[h(i + 'p', 2)]},
         'variantes': v}
    if 'auteur' in x:
        j['auteur'] = x['auteur']
    if i in SOURIS:
        j['commandes'] = 'souris'
    if i in AIDE:
        j['aide'] = AIDE[i]
    if i in DUREE:
        j['touche_duree'] = DUREE[i]
    if i in DELAI:
        j['touche_delai'] = DELAI[i]
    if i in SEQ:
        if SEQ[i]:
            j['sequence'] = SEQ[i]
    elif i in TOUCHES:
        t = TOUCHES[i]
        if t == 'esp':
            j['touche_type'] = 'espace'
        else:
            j['touche'] = t[-1]
            if t.startswith('c'):
                j['touche_type'] = 'chiffre'
    j['actif'] = i in ACTIFS
    if 'note' in x:
        j['note'] = x['note']
    elif len(d) > 2:
        j['note'] = 'Disquettes 3 et suivantes : F12 › Floppy disks pour changer.'
    elif len(d) == 2:
        j['note'] = 'Disquette 2 en B. Si le jeu la veut en A : F12 › Floppy disks.'
    jeux.append(j)

args = [a for a in sys.argv[1:] if not a.startswith('--')]
sortie = args[0] if args else 'catalogue.json'
cat = {
    'titre': 'LUDOTHÈQUE ST',
    'hatari': {'machine': 'ste', 'tos': 'tos/tos162fr.img', 'options': []},
    'vrac': False,
    'jeux': jeux,
}
if '--garder' in sys.argv and os.path.exists(sortie):
    ancien = json.load(open(sortie, encoding='utf-8'))
    coches = {a['id']: a.get('actif', True) for a in ancien.get('jeux', [])}
    for j in jeux:
        if j['id'] in coches:
            j['actif'] = coches[j['id']]
    cat['vrac'] = ancien.get('vrac', False)
json.dump(cat, open(sortie, 'w', encoding='utf-8', newline='\n'), ensure_ascii=False, indent=1)
print(len(jeux), 'jeux,', sum(1 for j in jeux if j['actif']), 'dans le bac')
