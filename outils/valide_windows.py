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
import json, os, sys, glob, time, subprocess, ctypes
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
        open(fi, 'w').write('screenshot %s\n' % os.path.join(d, 'v%05d.bmp' % k))
        cmds.append('b VBL = %d :trace :quiet :once :file %s' % (k, fi))
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
    for i, x in enumerate(v.get('disques', [])[:2]):
        args += ['--disk-a' if i == 0 else '--disk-b', disque_propre(os.path.join(PACK, x), v.get('correctifs'), d)]
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
