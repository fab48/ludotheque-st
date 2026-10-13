"""Test de bout en bout de l'interface (Chromium sans tête) contre le lanceur local."""
import sys, time
from playwright.sync_api import sync_playwright

URL = 'http://127.0.0.1:8480/'
OUT = sys.argv[1] if len(sys.argv) > 1 else '/tmp/ui'
import os; os.makedirs(OUT, exist_ok=True)

with sync_playwright() as p:
    b = p.chromium.launch(args=['--autoplay-policy=no-user-gesture-required'])
    pg = b.new_page(viewport={'width': 1280, 'height': 720})
    logs = []
    pg.on('console', lambda m: logs.append(m.type + ': ' + m.text))
    pg.on('pageerror', lambda e: logs.append('ERREUR: ' + str(e)))
    pg.goto(URL)
    time.sleep(4)
    pg.screenshot(path=f'{OUT}/1-bureau.png')
    for _ in range(3):
        pg.keyboard.press('ArrowRight'); time.sleep(0.15)
    time.sleep(0.6)
    pg.screenshot(path=f'{OUT}/2-nav.png')
    pg.keyboard.press('s'); time.sleep(0.6)   # saut à la lettre S
    pg.screenshot(path=f'{OUT}/3-lettre.png')
    pg.keyboard.press('F10'); time.sleep(0.5)
    pg.screenshot(path=f'{OUT}/4-panneau.png')
    pg.keyboard.press('Escape'); time.sleep(0.3)
    pg.mouse.click(160, 18); time.sleep(0.3)    # menu Affichage
    pg.screenshot(path=f'{OUT}/5-menu.png')
    pg.keyboard.press('Escape'); time.sleep(0.3)
    # charger Mouse Trap
    pg.keyboard.press('m'); time.sleep(0.6)
    titre = pg.text_content('#fa-titre')
    print('sélection :', titre)
    pg.keyboard.press('Enter'); time.sleep(1.2)
    pg.screenshot(path=f'{OUT}/6-lecture.png')
    time.sleep(3)
    pg.screenshot(path=f'{OUT}/7-enjeu.png')
    print('phase après chargement :', pg.evaluate('etat.phase'))
    with open(f'{OUT}/attente', 'w') as f:
        f.write('jeu')
    # le script appelant simule Échap maintenu, puis on vérifie le retour
    for _ in range(40):
        if pg.evaluate('etat.phase') == 'bureau':
            break
        time.sleep(0.5)
    time.sleep(1)
    print('phase après sortie :', pg.evaluate('etat.phase'))
    pg.screenshot(path=f'{OUT}/8-retour.png')
    # version web : Shufflepuck reforged
    pg.keyboard.press('s'); time.sleep(0.4)
    for _ in range(6):
        if 'Shufflepuck' in (pg.text_content('#fa-titre') or ''):
            break
        pg.keyboard.press('ArrowRight'); time.sleep(0.3)
    pg.keyboard.press('ArrowDown'); time.sleep(0.3)
    pg.screenshot(path=f'{OUT}/9-shuffle.png')
    pg.keyboard.press('Enter'); time.sleep(5)
    pg.screenshot(path=f'{OUT}/10-web.png')
    print('phase web :', pg.evaluate('etat.phase'))
    pg.evaluate("fermerWeb(false)"); time.sleep(1.5)
    print('phase après web :', pg.evaluate('etat.phase'))
    print('\n'.join(l for l in logs if 'ERREUR' in l or 'error' in l.lower())[:3000])
    b.close()
