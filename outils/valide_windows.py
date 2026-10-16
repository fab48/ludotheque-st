"""Test des jeux du catalogue sous Windows, sans toucher à l'écran : comme le lanceur
(mêmes arguments que preparerHatari, turbo « disque » de notre Hatari, mêmes touches de
démarrage que sequenceDemarrage), Hatari en fenêtre hors écran sans prendre le focus,
touches envoyées par PostMessage, une capture toutes les 250 VBL par le débogueur.
Résultat : runtime/tests/<id>/v*.bmp et des planches runtime/tests/planche-NN.png
(6 jeux par planche, 8 images par jeu).
  python outils/valide_windows.py [id ...]        (sans id : tout le catalogue)
  VMAX=4500 : durée en VBL (50 par seconde).  PAR=8 : jeux testés en même temps.
  MHZ=16 : processeur à 16 MHz (résultats dans runtime/tests-16mhz).
Demande Pillow (pip install pillow).
"""
import json, os, sys, glob, time, struct, subprocess, ctypes
from ctypes import wintypes
from concurrent.futures import ThreadPoolExecutor
from PIL import Image, ImageDraw

PACK = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
HATARI = os.path.join(PACK, 'emulateurs', 'hatari-windows', 'hatari.exe')
MHZ = int(os.environ.get('MHZ', '8'))
SCR = os.path.join(PACK, 'runtime', 'tests' if MHZ == 8 else 'tests-%dmhz' % MHZ)

u32 = ctypes.WinDLL('user32', use_last_error=True)
EnumProc = ctypes.WINFUNCTYPE(wintypes.BOOL, wintypes.HWND, wintypes.LPARAM)
u32.EnumWindows.argtypes = [EnumProc, wintypes.LPARAM]
u32.PostMessageW.argtypes = [wintypes.HWND, wintypes.UINT, wintypes.WPARAM, wintypes.LPARAM]
u32.SetWindowPos.argtypes = [wintypes.HWND, wintypes.HWND, ctypes.c_int, ctypes.c_int, ctypes.c_int, ctypes.c_int, wintypes.UINT]
u32.GetWindowThreadProcessId.argtypes = [wintypes.HWND, ctypes.POINTER(wintypes.DWORD)]
u32.IsWindowVisible.argtypes = [wintypes.HWND]
u32.MapVirtualKeyW.restype = wintypes.UINT


def fenetres(pid):
    out = []

    def cb(h, l):
        p = wintypes.DWORD()
        u32.GetWindowThreadProcessId(h, ctypes.byref(p))
        if p.value == pid and u32.IsWindowVisible(h):
            out.append(h)
        return True
    u32.EnumWindows(EnumProc(cb), 0)
    return out


PAVE = {'0': 0x52, '1': 0x4F, '2': 0x50, '3': 0x51, '4': 0x4B, '5': 0x4C, '6': 0x4D, '7': 0x47, '8': 0x48, '9': 0x49}
VKPAVE = {'0': 0x60, '1': 0x61, '2': 0x62, '3': 0x63, '4': 0x64, '5': 0x65, '6': 0x66, '7': 0x67, '8': 0x68, '9': 0x69}


def touche(h, sc, vk=None, ext=False, duree=0.06):
    if vk is None:
        vk = u32.MapVirtualKeyW(sc, 1)  # MAPVK_VSC_TO_VK
    l = 1 | (sc << 16) | ((1 << 24) if ext else 0)
    u32.PostMessageW(h, 0x100, vk, l)
    time.sleep(duree)
    u32.PostMessageW(h, 0x101, vk, l | (1 << 30) | (1 << 31))


VMAX, PAS = int(os.environ.get("VMAX", "3500")), 250


def code(nom):
    n = nom.strip().lower()
    if n == 'espace':
        return 0x39, 0x20
    if n == 'entree':
        return 0x1C, 0x0D
    if n == 'echap':
        return 0x01, 0x1B
    if n.startswith('pave'):
        return PAVE[n[4:]], VKPAVE[n[4:]]
    if n[0] == 'f' and n[1:].isdigit():
        return 0x3A + int(n[1:]), 0x6F + int(n[1:])
    if len(n) == 1 and n.isdigit():
        return 0x02 + '1234567890'.index(n), ord(n)
    for l, b in (('qwertyuiop', 0x10), ('asdfghjkl', 0x1E), ('zxcvbnm', 0x2C)):
        if n in l:
            return b + l.index(n), ord(n.upper())
    raise ValueError(nom)


def touches_jeu(j):
    noms = j.get('sequence')
    if not noms:
        if j.get('touche_type') == 'espace':
            noms = ['espace']
        elif not j.get('touche'):
            return []
        elif j.get('touche_type') == 'chiffre':
            noms = [j['touche']]
        else:
            noms = ['pave' + j['touche']]
    return [code(n) for n in noms]


def disque_propre(p, cor, d):
    if not p.lower().endswith('.st') and not cor:
        return p
    b = bytearray(open(p, 'rb').read())
    lgd = p.lower().endswith('.st') and b'Little Green Desktop' in b[:512]
    if not lgd and not cor:
        return p
    if lgd:
        b[511] ^= 1
    for c in cor or []:
        b = bytearray(bytes(b).replace(bytes.fromhex(c['cherche']), bytes.fromhex(c['remplace'])))
    out = os.path.join(d, 'propre-' + os.path.basename(p))
    open(out, 'wb').write(b)
    return out


def fichiers_disquette(p):
    """Fichiers et dossiers d'une image .st/.msa (FAT12), comme fichiersDisquette du lanceur."""
    d = open(p, 'rb').read()
    if p.lower().endswith('.msa'):
        if len(d) < 10 or d[:2] != b'\x0e\x0f':
            return []
        spt, faces, deb, fin = struct.unpack_from('>HHHH', d, 2)
        out, o = bytearray(), 10
        for _ in range((fin - deb + 1) * (faces + 1)):
            n = struct.unpack_from('>H', d, o)[0]
            blk, o = d[o + 2:o + 2 + n], o + 2 + n
            if n == spt * 512:
                out += blk
                continue
            i = 0
            while i < len(blk):
                if blk[i] == 0xe5 and i + 3 < len(blk):
                    out += bytes([blk[i + 1]]) * struct.unpack_from('>H', blk, i + 2)[0]
                    i += 4
                else:
                    out.append(blk[i])
                    i += 1
        d = bytes(out)
    elif not p.lower().endswith('.st'):
        return []
    bps, spc, res, nfat, nroot = struct.unpack_from('<HBHBH', d, 11)
    spf = struct.unpack_from('<H', d, 22)[0]
    if bps != 512 or not spc or not res or not 1 <= nfat <= 2 or not 0 < nroot <= 1024 or not 0 < spf <= 16:
        return []
    fat, racine = d[res * 512:(res + spf) * 512], (res + nfat * spf) * 512
    donnees = racine + nroot * 32

    def lire(c):
        out, n = b'', 0
        while 2 <= c < 0xff0 and n < 64:
            out += d[donnees + (c - 2) * spc * 512:donnees + (c - 1) * spc * 512]
            v = fat[c * 3 // 2] | fat[c * 3 // 2 + 1] << 8
            c, n = (v >> 4) if c & 1 else (v & 0xfff), n + 1
        return out

    def valide(e):
        for k, part in ((0, e[:8]), (1, e[8:11])):
            s = part.decode('latin1')
            if (k == 0 and s[0] == ' ') or s.rstrip(' ') != s.rstrip(' ').replace(' ', '') \
                    or any(not 0x21 <= ord(c) <= 0x7e or c in '"*+,/:;<=>?[\\]|' for c in s.rstrip(' ')) \
                    or ('.' in s and s.rstrip(' ') not in ('.', '..')):
                return False
        return True

    def parcourir(raw, pre, prof):
        noms = []
        for i in range(0, len(raw) - 31, 32):
            e = raw[i:i + 32]
            if e[0] == 0:
                break
            if e[0] == 0xe5 or e[11] & 8:
                continue
            if not valide(e[:11]) or e[11] & 0xc0:
                return None
            n = e[:8].decode('latin1').rstrip() + ('.' + e[8:11].decode('latin1').rstrip() if e[8:11].strip() else '')
            if n in ('.', '..', ''):
                continue
            noms.append(pre + n.upper())
            if e[11] & 0x10 and prof < 8:
                noms += parcourir(lire(struct.unpack_from('<H', e, 26)[0]), pre + n.upper() + '\\', prof + 1) or []
        return noms
    return parcourir(d[racine:donnees], '', 0) or []


def liste_disquettes(images, d):
    """runtime/tests/<id>/disques.txt pour HATARI_LUDO_DISQUES (changement automatique), ou None."""
    if len(images) < 2:
        return None
    lignes, lisibles = ['# disquettes du jeu'], 0
    for p in images:
        noms = fichiers_disquette(p)
        lisibles += bool(noms)
        lignes += ['disque ' + p] + noms
    if not lisibles:
        return None
    sortie = os.path.join(d, 'disques.txt')
    open(sortie, 'w', encoding='latin1').write('\n'.join(lignes) + '\n')
    return sortie


def un(cat, j):
    v = j['variantes'][0]
    if v['type'] != 'hatari':
        return j['id'], 'non hatari'
    d = os.path.join(SCR, j['id'])
    os.makedirs(d, exist_ok=True)
    for f in glob.glob(os.path.join(d, 'v*.bmp')):
        os.remove(f)
    cmds = []
    for k in range(PAS, VMAX, PAS):
        fi = os.path.join(d, 'c%05d.ini' % k)
        # noms relatifs (Hatari tourne dans d) : le débogueur coupe les chemins aux espaces
        open(fi, 'w').write('screenshot v%05d.bmp\n' % k)
        cmds.append('b VBL = %d :trace :quiet :once :file %s' % (k, os.path.basename(fi)))
    parse = os.path.join(d, 'debug.ini')
    open(parse, 'w').write('\n'.join(cmds) + '\n')
    cfg = os.path.join(d, 'test.cfg')
    open(cfg, 'w').write('[Log]\nnAlertDlgLogLevel = 0\nbConfirmQuit = FALSE\n[Sound]\nbEnableSound = FALSE\n'
                         '[Joystick1]\nkUp = Up\nkDown = Down\nkLeft = Left\nkRight = Right\nkFire = Right Ctrl\n'
                         '[Joystick0]\nkUp = Z\nkDown = S\nkLeft = Q\nkRight = D\nkFire = Left Ctrl\n')
    machine = v.get('machine') or cat['hatari']['machine'] or 'ste'
    tos = v.get('tos') or cat['hatari']['tos']
    if machine in ('st', 'megast') and ('162' in tos or '106' in tos):
        machine = 'ste'
    args = [HATARI, '--configfile', cfg, '--machine', machine, '--alert-level', 'fatal',
            '--tos', os.path.join(PACK, tos)]
    images = [disque_propre(os.path.join(PACK, x), v.get('correctifs'), d) for x in v.get('disques', [])]
    for i, x in enumerate(images[:2]):
        args += ['--disk-a' if i == 0 else '--disk-b', x]
    args += ['--confirm-quit', 'no', '--statusbar', 'no', '--joy1', 'keys']
    if j.get('commandes') != 'souris':
        args += ['--joy0', 'keys']
    args += ['--fastfdc', 'yes', '--fast-boot', 'yes', '--drive-led', 'no', '--borders', 'no', '--zoom', '1',
             '--sound', 'off', '--parse', parse, '--run-vbls', str(VMAX), '--log-file', os.path.join(d, 'hatari.log')]
    if v.get('dossier'):
        args += ['--harddrive', os.path.join(PACK, v['dossier']), '--gemdos-drive', 'C']
    if v.get('auto'):
        args += ['--auto', v['auto']]
    args += cat['hatari'].get('options', []) + v.get('options', [])
    if MHZ != 8:
        args += ['--cpuclock', str(MHZ)]
    env = dict(os.environ, SDL_AUDIODRIVER='dummy', SDL_WINDOW_NO_ACTIVATION_WHEN_SHOWN='1', HOME=d,
               HATARI_LUDO_TURBO='disque')
    liste = liste_disquettes(images, d)
    if liste:
        env['HATARI_LUDO_DISQUES'] = liste
    t0 = time.time()
    p = subprocess.Popen(args, cwd=d, env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                         stderr=subprocess.DEVNULL)
    h = None
    for _ in range(200):
        w = fenetres(p.pid)
        if w:
            h = w[0]
            u32.SetWindowPos(h, 1, -3000, -3000, 0, 0, 0x0001 | 0x0010)
            break
        time.sleep(0.05)
    ts = touches_jeu(j)
    if ts and h:
        time.sleep(0.4 + (j.get('touche_delai') or 0))
        debut, k = time.time(), 0
        while time.time() - debut < (j.get('touche_duree') or 5) and p.poll() is None:
            sc, vk = ts[k % len(ts)]
            touche(h, sc, vk, duree=0.12)
            time.sleep(0.25 + 0.06)
            k += 1
    try:
        p.wait(timeout=VMAX / 50 + 40)
        res = 'ok'
    except subprocess.TimeoutExpired:
        p.kill()
        res = 'TIMEOUT'
    n = len(glob.glob(os.path.join(d, 'v*.bmp')))
    return j['id'], n, round(time.time() - t0), res


def resume(ids, sortie, par=6, n=8, w=240, h=150):
    for k in range(0, len(ids), par):
        lot = ids[k:k + par]
        P = Image.new('RGB', (n * w, len(lot) * (h + 16)), (60, 0, 60))
        dr = ImageDraw.Draw(P)
        for r, i in enumerate(lot):
            ims = sorted(glob.glob(os.path.join(SCR, i, 'v*.bmp')))
            if len(ims) > n:
                ims = [ims[round(x * (len(ims) - 1) / (n - 1))] for x in range(n)]
            y = r * (h + 16)
            dr.text((4, y + 2), i, fill=(255, 255, 0))
            for c, f in enumerate(ims):
                try:
                    P.paste(Image.open(f).convert('RGB').resize((w, h)), (c * w, y + 16))
                except Exception:
                    pass
        P.save('%s-%02d.png' % (sortie, k // par))


if __name__ == '__main__':
    cat = json.load(open(os.path.join(PACK, 'catalogue.json'), encoding='utf-8'))
    os.makedirs(SCR, exist_ok=True)
    sortie = os.path.join(SCR, 'planche')
    ids = sys.argv[1:]
    jeux = [j for j in cat['jeux'] if not ids or j['id'] in ids]
    with ThreadPoolExecutor(int(os.environ.get('PAR', '8'))) as ex:
        for r in ex.map(lambda j: un(cat, j), jeux):
            print(r, flush=True)
    resume([j['id'] for j in jeux], sortie)
