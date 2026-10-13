#!/usr/bin/env python3
"""Installe les images de disquettes et les TOS de la ludothèque depuis la collection
TOSEC Atari ST (2012-04-23), d'après outils/images_tosec.json.

  python outils/installer_images.py "<dossier de la collection TOSEC>"
  (Windows : glisser le dossier sur « Installer les images TOSEC.bat »)

Chaque image est extraite de son zip, contrôlée (CRC32 et taille) et rangée sous le
nom qu'attend catalogue.json (disques/…, tos/…). Une image déjà présente n'est jamais
remplacée : elle peut contenir les sauvegardes d'un jeu. Si un zip n'est pas à sa
place (autre rangement, autre version de TOSEC), il est cherché par son nom, puis par
le CRC de son contenu dans tous les zips du dossier.
Options : --pack <dossier> (installer ailleurs), --verifier (ne rien écrire).

Entretien (après avoir ajouté un jeu au catalogue) :
  python outils/installer_images.py --carte "<dossier TOSEC>"
refait images_tosec.json : chaque image du catalogue présente dans le pack est
retrouvée dans la collection par son contenu (CRC32 + taille, quelques minutes) ;
une image modifiée (sauvegardes) garde la source déjà notée.
"""
import json, os, sys, zipfile, zlib

try:
    sys.stdout.reconfigure(errors='replace')
except Exception:
    pass

ICI = os.path.dirname(os.path.abspath(__file__))


def crc(b):
    return '%08x' % (zlib.crc32(b) & 0xffffffff)


def images_du_catalogue(pack):
    cat = json.load(open(os.path.join(pack, 'catalogue.json'), encoding='utf-8'))
    voulus = {cat['hatari']['tos'], 'tos/tos102fr.img', 'tos/tos104fr.img', 'tos/tos206fr.img'}
    for j in cat['jeux']:
        for v in j['variantes']:
            voulus.update(v.get('disques', []))
            for k in ('disque', 'tos'):
                if v.get(k):
                    voulus.add(v[k])
    return sorted(x for x in voulus if x)


def carte(pack, rac):
    fichier = os.path.join(ICI, 'images_tosec.json')
    ancien = json.load(open(fichier, encoding='utf-8'))['images'] if os.path.exists(fichier) else {}
    locaux, sans_fichier = {}, []
    for p in images_du_catalogue(pack):
        f = os.path.join(pack, *p.split('/'))
        if not os.path.exists(f):
            sans_fichier.append(p)
            continue
        b = open(f, 'rb').read()
        locaux.setdefault((crc(b), len(b)), []).append(p)
    print('Lecture des zips de la collection...')
    trouves = {}
    for d, _, fs in os.walk(rac):
        for x in fs:
            if not x.lower().endswith('.zip'):
                continue
            z = os.path.join(d, x)
            try:
                infos = zipfile.ZipFile(z).infolist()
            except Exception:
                continue
            for i in infos:
                for p in locaux.get(('%08x' % i.CRC, i.file_size), []):
                    trouves.setdefault(p, {'zip': os.path.relpath(z, rac).replace(os.sep, '/'), 'membre': i.filename,
                                           'crc32': '%08x' % i.CRC, 'taille': i.file_size})
    images, sans_source = {}, []
    for p in sorted([q for v in locaux.values() for q in v] + sans_fichier):
        if p in trouves:
            images[p] = trouves[p]
        elif p in ancien:
            images[p] = ancien[p]
        else:
            sans_source.append(p)
    json.dump({'collection': 'TOSEC Atari ST 2012-04-23', 'images': images},
              open(fichier, 'w', encoding='utf-8', newline='\n'), ensure_ascii=False, indent=1)
    print('%d images notées dans images_tosec.json (%d retrouvées par leur contenu).' % (len(images), len(trouves)))
    if sans_source:
        print('Sans source TOSEC (%d) :' % len(sans_source))
        for p in sans_source:
            print('   ', p)
    return 0


def main():
    args = sys.argv[1:]
    if args[:1] == ['--carte'] and len(args) > 1 and os.path.isdir(args[1]):
        return carte(os.path.dirname(ICI), os.path.abspath(args[1]))
    pack = os.path.dirname(ICI)
    verifier = '--verifier' in args
    if '--pack' in args:
        k = args.index('--pack')
        pack = os.path.abspath(args[k + 1])
        del args[k:k + 2]
    args = [a for a in args if a != '--verifier']
    if not args or not os.path.isdir(args[0]):
        print(__doc__)
        print('Dossier de la collection TOSEC introuvable.' if args else 'Indiquez le dossier de la collection TOSEC.')
        return 1
    rac = os.path.abspath(args[0])
    images = json.load(open(os.path.join(ICI, 'images_tosec.json'), encoding='utf-8'))['images']

    a_faire, deja, modifiees = {}, 0, []
    for dest, src in images.items():
        f = os.path.join(pack, *dest.split('/'))
        if os.path.exists(f):
            b = open(f, 'rb').read()
            if crc(b) == src['crc32'] and len(b) == src['taille']:
                deja += 1
            else:
                modifiees.append(dest)
        else:
            a_faire[dest] = src
    print('%d images au catalogue : %d déjà là, %d à installer.' % (len(images), deja + len(modifiees), len(a_faire)))
    if modifiees:
        print('%d images présentes diffèrent de TOSEC (sauvegardes, autre version) : gardées.' % len(modifiees))

    # zips de la collection, indexés par nom (au cas où le rangement diffère)
    par_nom = None

    def trouver_zip(rel):
        nonlocal par_nom
        p = os.path.join(rac, *rel.split('/'))
        if os.path.exists(p):
            return p
        if par_nom is None:
            par_nom = {}
            for d, _, fs in os.walk(rac):
                for x in fs:
                    if x.lower().endswith('.zip'):
                        par_nom.setdefault(x.lower(), os.path.join(d, x))
        return par_nom.get(os.path.basename(rel).lower())

    def extraire(z, membre, attendu):
        with zipfile.ZipFile(z) as zz:
            noms = [i for i in zz.infolist() if i.filename == membre or
                    ('%08x' % i.CRC == attendu['crc32'] and i.file_size == attendu['taille'])]
            if not noms:
                return None
            b = zz.read(noms[0])
        return b if crc(b) == attendu['crc32'] and len(b) == attendu['taille'] else None

    installees, introuvables = 0, []
    for dest, src in sorted(a_faire.items()):
        z = trouver_zip(src['zip'])
        b = extraire(z, src['membre'], src) if z else None
        if b is None:
            introuvables.append(dest)
            continue
        if not verifier:
            f = os.path.join(pack, *dest.split('/'))
            os.makedirs(os.path.dirname(f), exist_ok=True)
            open(f, 'wb').write(b)
        installees += 1

    # dernier recours : le contenu (CRC) dans tous les zips de la collection
    if introuvables:
        print('Recherche par contenu de %d images dans toute la collection (quelques minutes)...' % len(introuvables))
        voulu = {(images[d]['crc32'], images[d]['taille']): d for d in introuvables}
        for d, _, fs in os.walk(rac):
            for x in fs:
                if not voulu or not x.lower().endswith('.zip'):
                    continue
                z = os.path.join(d, x)
                try:
                    infos = zipfile.ZipFile(z).infolist()
                except Exception:
                    continue
                for i in infos:
                    k = ('%08x' % i.CRC, i.file_size)
                    if k in voulu:
                        dest = voulu.pop(k)
                        b = extraire(z, i.filename, images[dest])
                        if b is not None:
                            if not verifier:
                                f = os.path.join(pack, *dest.split('/'))
                                os.makedirs(os.path.dirname(f), exist_ok=True)
                                open(f, 'wb').write(b)
                            installees += 1
                            introuvables.remove(dest)
    print('%d images %s.' % (installees, 'trouvées (rien écrit : --verifier)' if verifier else 'installées'))
    if introuvables:
        print('Introuvables dans cette collection (%d) : ces jeux restent hors du bac.' % len(introuvables))
        for d in introuvables:
            print('   ', d)
    else:
        print('Tout est là : relancez la ludothèque.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
