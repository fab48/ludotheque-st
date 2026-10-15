//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	pGetAsyncKey     = user32.NewProc("GetAsyncKeyState")
	pEnumWindows     = user32.NewProc("EnumWindows")
	pGetWinThreadPid = user32.NewProc("GetWindowThreadProcessId")
	pIsWindowVisible = user32.NewProc("IsWindowVisible")
	pSetForeground   = user32.NewProc("SetForegroundWindow")
	pBringToTop      = user32.NewProc("BringWindowToTop")
	pShowWindow      = user32.NewProc("ShowWindow")
	pKeybdEvent      = user32.NewProc("keybd_event")
	pGetWindowText   = user32.NewProc("GetWindowTextW")
	pPostMessage     = user32.NewProc("PostMessageW")

	xinput      = chargerXInput()
	pXInputGet  *syscall.LazyProc
	winmm       = syscall.NewLazyDLL("winmm.dll")
	pJoyGetPosE = winmm.NewProc("joyGetPosEx")
)

func chargerXInput() *syscall.LazyDLL {
	for _, n := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
		d := syscall.NewLazyDLL(n)
		if d.Load() == nil {
			return d
		}
	}
	return nil
}

var navCmd *exec.Cmd

func preparerProcessus(cmd *exec.Cmd) {
	// pas de fenêtre de console pour les programmes en mode console
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
}

// cheminCourt : nom court Windows (8.3, tout en ASCII) d'un dossier dont le nom a
// des caractères hors ASCII ; Hatari ne sait pas ouvrir les autres. Le dossier tel
// quel si le disque ne garde pas de noms courts.
func cheminCourt(p string) string {
	ascii := true
	for _, r := range p {
		if r > 127 {
			ascii = false
			break
		}
	}
	long, err := syscall.UTF16PtrFromString(p)
	if ascii || err != nil {
		return p
	}
	buf := make([]uint16, 1024)
	n, err := syscall.GetShortPathName(long, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return p
	}
	court := syscall.UTF16ToString(buf[:n])
	for _, r := range court {
		if r > 127 {
			log.Printf("dossier sans nom court en ASCII : Hatari risque de ne pas trouver ses fichiers (%s)", p)
			return p
		}
	}
	return court
}

// ---------- navigateur ----------

func trouverNavigateur() (string, string) {
	if cfg.Navigateur != "" && cfg.Navigateur != "auto" {
		p, err := chemin(cfg.Navigateur)
		if err == nil && existe(p) {
			return p, famille(p)
		}
	}
	// Chromium portable déposé dans le pack
	for _, c := range []string{"navigateur/chrome.exe", "navigateur/chrome-win/chrome.exe", "navigateur/chromium/chrome.exe"} {
		if p, _ := chemin(c); existe(p) {
			return p, "chromium"
		}
	}
	pf := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")}
	for _, d := range pf {
		if d == "" {
			continue
		}
		for _, rel := range []string{`Google\Chrome\Application\chrome.exe`, `Chromium\Application\chrome.exe`,
			`BraveSoftware\Brave-Browser\Application\brave.exe`} {
			if p := filepath.Join(d, rel); existe(p) {
				return p, "chromium"
			}
		}
	}
	for _, d := range pf {
		if p := filepath.Join(d, `Microsoft\Edge\Application\msedge.exe`); d != "" && existe(p) {
			return p, "edge"
		}
	}
	for _, d := range pf {
		if p := filepath.Join(d, `Mozilla Firefox\firefox.exe`); d != "" && existe(p) {
			return p, "firefox"
		}
	}
	return "", ""
}

func famille(p string) string {
	b := strings.ToLower(filepath.Base(p))
	switch {
	case strings.Contains(b, "firefox"):
		return "firefox"
	case strings.Contains(b, "msedge"):
		return "edge"
	}
	return "chromium"
}

func lancerNavigateur(url string) *exec.Cmd {
	exe, fam := trouverNavigateur()
	if exe == "" {
		return nil
	}
	args := argumentsNavigateur(fam, url)
	navCmd = exec.Command(exe, args...)
	log.Printf("navigateur : %s %q", exe, args)
	if err := navCmd.Start(); err != nil {
		log.Printf("navigateur : %v", err)
		return nil
	}
	return navCmd
}

func fermerNavigateur() {
	if navCmd != nil && navCmd.Process != nil {
		_ = navCmd.Process.Kill()
	}
}

func ouvrirParDefaut(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

// ---------- premier plan ----------

// Les rappels Windows sont limités en nombre : on les crée une seule fois.
var (
	muEnum      sync.Mutex
	enumPid     int
	enumTrouves []uintptr
	enumTitre   string
	enumFenetre uintptr
	rappelPid   = syscall.NewCallback(func(h uintptr, _ uintptr) uintptr {
		var p uint32
		pGetWinThreadPid.Call(h, uintptr(unsafe.Pointer(&p)))
		if int(p) == enumPid {
			if v, _, _ := pIsWindowVisible.Call(h); v != 0 {
				enumTrouves = append(enumTrouves, h)
			}
		}
		return 1
	})
	rappelTitre = syscall.NewCallback(func(h uintptr, _ uintptr) uintptr {
		if v, _, _ := pIsWindowVisible.Call(h); v == 0 {
			return 1
		}
		buf := make([]uint16, 256)
		pGetWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), 256)
		if strings.Contains(syscall.UTF16ToString(buf), enumTitre) {
			enumFenetre = h
			return 0
		}
		return 1
	})
)

func fenetresDuProcessus(pid int) []uintptr {
	muEnum.Lock()
	defer muEnum.Unlock()
	enumPid, enumTrouves = pid, nil
	pEnumWindows.Call(rappelPid, 0)
	return append([]uintptr(nil), enumTrouves...)
}

func fenetreParTitre(morceau string) uintptr {
	muEnum.Lock()
	defer muEnum.Unlock()
	enumTitre, enumFenetre = morceau, 0
	pEnumWindows.Call(rappelTitre, 0)
	return enumFenetre
}

// entreeSimulee : jusqu'à quand (UnixNano) une entrée vient de nos propres touches
// simulées (Alt de forcerPremierPlan, touches de démarrage) et non de l'utilisateur ;
// sinon l'écran de veille s'arrêterait aussitôt la démo mise au premier plan.
var entreeSimulee atomic.Int64

func simuler() { entreeSimulee.Store(time.Now().Add(500 * time.Millisecond).UnixNano()) }

func forcerPremierPlan(h uintptr) {
	// Astuce classique : un appui d'Alt simulé autorise SetForegroundWindow.
	simuler()
	pKeybdEvent.Call(0x12, 0, 0, 0)
	pKeybdEvent.Call(0x12, 0, 2, 0)
	pShowWindow.Call(h, 5)
	pBringToTop.Call(h)
	pSetForeground.Call(h)
}

func premierPlan(pid int) {
	for i := 0; i < 40; i++ {
		time.Sleep(150 * time.Millisecond)
		if hs := fenetresDuProcessus(pid); len(hs) > 0 {
			forcerPremierPlan(hs[0])
			if i > 8 {
				return
			}
		}
	}
}

// fermerDoucement : WM_CLOSE aux fenêtres du jeu (Hatari sort proprement et
// réécrit les disquettes modifiées).
func fermerDoucement(pid int) {
	for _, h := range fenetresDuProcessus(pid) {
		pPostMessage.Call(h, 0x0010, 0, 0)
	}
}

func revenirNavigateur() {
	time.Sleep(200 * time.Millisecond)
	if h := fenetreParTitre("Ludothèque ST"); h != 0 {
		forcerPremierPlan(h)
	}
}

// ---------- outils communs ----------

func compterManettes() int {
	debut := time.Now()
	defer func() { // certains pilotes de manette rendent winmm très lent
		if d := time.Since(debut); d > 300*time.Millisecond {
			log.Printf("manettes : comptées en %s (lent)", d.Round(time.Millisecond))
		}
	}()
	n := 0
	if xinput != nil {
		if pXInputGet == nil {
			pXInputGet = xinput.NewProc("XInputGetState")
		}
		for i := 0; i < 4; i++ {
			var e xinputEtat
			if r, _, _ := pXInputGet.Call(uintptr(i), uintptr(unsafe.Pointer(&e))); r == 0 {
				n++
			}
		}
	}
	if n > 0 {
		return n
	}
	for i := 0; i < 16; i++ { // manettes DirectInput (winmm)
		ji := joyInfoEx{Flags: 0x80}
		ji.Size = uint32(unsafe.Sizeof(ji))
		if r, _, _ := pJoyGetPosE.Call(uintptr(i), uintptr(unsafe.Pointer(&ji))); r == 0 {
			n++
		}
	}
	return n
}

// envoyerTouche : Windows simule la touche (scancode PC) dans la fenêtre de Hatari.
func envoyerTouche(fifo string, st, pc, pid int) {
	if hs := fenetresDuProcessus(pid); len(hs) > 0 {
		forcerPremierPlan(hs[0])
		time.Sleep(60 * time.Millisecond)
	} else {
		return // pas de fenêtre Hatari : ne rien taper dans le navigateur
	}
	const scancode, relache = 0x0008, 0x0002
	simuler()
	pKeybdEvent.Call(0, uintptr(pc), scancode, 0)
	time.Sleep(120 * time.Millisecond)
	pKeybdEvent.Call(0, uintptr(pc), scancode|relache, 0)
}

// attendreFenetre : la fenêtre de Hatari doit exister avant d'y envoyer des touches.
func attendreFenetre(pid int) {
	for i := 0; i < 50; i++ {
		if hs := fenetresDuProcessus(pid); len(hs) > 0 {
			forcerPremierPlan(hs[0])
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// finTurbo : F11, raccourci « accéléré » de Hatari dans la config du pack
// (Arrêt défil, touche à bascule, n'arrivait pas jusqu'à Hatari sous Windows).
func finTurbo(fifo string, pid int) { envoyerTouche("", 0, 0x57, pid) }

// ---------- surveillance : Échap maintenu, Select+Start ----------

type xinputEtat struct {
	Paquet  uint32
	Boutons uint16
	LT, RT  uint8
	LX, LY  int16
	RX, RY  int16
}

type joyInfoEx struct {
	Size, Flags, X, Y, Z, R, U, V, Buttons, ButtonNumber, POV, R1, R2 uint32
}

// boutonsSelectStart : Select (Back) et Start, toutes manettes confondues.
func boutonsSelectStart() (sel, start bool) {
	if xinput != nil && pXInputGet != nil {
		for i := 0; i < 4; i++ {
			var e xinputEtat
			if r, _, _ := pXInputGet.Call(uintptr(i), uintptr(unsafe.Pointer(&e))); r == 0 {
				sel = sel || e.Boutons&0x20 != 0
				start = start || e.Boutons&0x10 != 0
			}
		}
	}
	for i := 0; i < 4; i++ {
		ji := joyInfoEx{Flags: 0x80}
		ji.Size = uint32(unsafe.Sizeof(ji))
		if r, _, _ := pJoyGetPosE.Call(uintptr(i), uintptr(unsafe.Pointer(&ji))); r != 0 {
			continue
		}
		for _, p := range cfg.BoutonsManette {
			sel = sel || ji.Buttons&(1<<uint(p[0])) != 0
			start = start || ji.Buttons&(1<<uint(p[1])) != 0
		}
	}
	return
}

// boutonTir : bouton A (XInput) ou premier bouton (DirectInput), toutes manettes.
func boutonTir() bool {
	if xinput != nil && pXInputGet != nil {
		for i := 0; i < 4; i++ {
			var e xinputEtat
			if r, _, _ := pXInputGet.Call(uintptr(i), uintptr(unsafe.Pointer(&e))); r == 0 && e.Boutons&0x1000 != 0 {
				return true
			}
		}
	}
	for i := 0; i < 4; i++ {
		ji := joyInfoEx{Flags: 0x80}
		ji.Size = uint32(unsafe.Sizeof(ji))
		if r, _, _ := pJoyGetPosE.Call(uintptr(i), uintptr(unsafe.Pointer(&ji))); r == 0 && ji.Buttons&1 != 0 {
			return true
		}
	}
	return false
}

func manetteCombo() bool {
	if xinput != nil {
		if pXInputGet == nil {
			pXInputGet = xinput.NewProc("XInputGetState")
		}
		for i := 0; i < 4; i++ {
			var e xinputEtat
			r, _, _ := pXInputGet.Call(uintptr(i), uintptr(unsafe.Pointer(&e)))
			if r == 0 && e.Boutons&0x0030 == 0x0030 { // BACK + START
				return true
			}
		}
	}
	// manettes DirectInput (via winmm)
	for i := 0; i < 4; i++ {
		ji := joyInfoEx{Flags: 0x80} // JOY_RETURNBUTTONS
		ji.Size = uint32(unsafe.Sizeof(ji))
		r, _, _ := pJoyGetPosE.Call(uintptr(i), uintptr(unsafe.Pointer(&ji)))
		if r != 0 {
			continue
		}
		for _, p := range cfg.BoutonsManette {
			if ji.Buttons&(1<<uint(p[0])) != 0 && ji.Buttons&(1<<uint(p[1])) != 0 {
				return true
			}
		}
	}
	return false
}

func surveillerEntrees() {
	if xinput != nil && pXInputGet == nil {
		pXInputGet = xinput.NewProc("XInputGetState")
	}
	var echap, combo time.Duration
	const pas = 50 * time.Millisecond
	var paquets [4]uint32
	var suivi suiviTouches
	var lecEsc, lecEsp bool
	for range time.Tick(pas) {
		if activiteSysteme() && time.Now().UnixNano() > entreeSimulee.Load() {
			signalerActivite()
		}
		if xinput != nil && pXInputGet != nil {
			for i := 0; i < 4; i++ {
				var e xinputEtat
				if r, _, _ := pXInputGet.Call(uintptr(i), uintptr(unsafe.Pointer(&e))); r == 0 {
					if paquets[i] != 0 && e.Paquet != paquets[i] {
						signalerActivite()
					}
					paquets[i] = e.Paquet
				}
			}
		}
		if jeux.EnJeu() == "demo" && demoEnLecture() {
			// Play Demo Random : Échap ou Select + Start pour revenir, Espace ou tir pour la suivante
			k, _, _ := pGetAsyncKey.Call(0x1B)
			e, _, _ := pGetAsyncKey.Call(0x20)
			esc, esp := k&0x8000 != 0, e&0x8000 != 0 || boutonTir()
			if esc && !lecEsc {
				arreterDemos("Échap")
			} else if manetteCombo() {
				arreterDemos("Select+Start")
			} else if esp && !lecEsp {
				demoSuivante()
			}
			lecEsc, lecEsp = esc, esp
			echap, combo = 0, 0
			continue
		}
		lecEsc, lecEsp = true, true // un appui en cours au lancement ne compte pas
		if jeux.EnJeu() == "libre" || jeux.EnJeu() == "demo" {
			echap, combo = 0, 0
			continue
		}
		p, _, _ := pGetAsyncKey.Call(0x13)
		sel, start := boutonsSelectStart()
		suivi.maj(p&0x8000 != 0, sel, start)
		if k, _, _ := pGetAsyncKey.Call(0x1B); k&0x8000 != 0 {
			echap += pas
		} else {
			echap = 0
		}
		if manetteCombo() {
			combo += pas
		} else {
			combo = 0
		}
		if cfg.SortieEchap > 0 && echap >= time.Duration(cfg.SortieEchap*float64(time.Second)) {
			echap = 0
			quitterJeu("Échap maintenu")
		}
		if cfg.SortieManette > 0 && combo >= time.Duration(cfg.SortieManette*float64(time.Second)) {
			combo = 0
			quitterJeu("Select+Start")
		}
	}
}
