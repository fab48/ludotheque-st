//go:build linux

package main

import (
	"encoding/binary"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

var navCmd *exec.Cmd

func preparerProcessus(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// bibliothèques livrées avec le Hatari du pack
	if d := filepath.Dir(cmd.Path); existe(filepath.Join(d, "libSDL2-2.0.so.0")) {
		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		cmd.Env = append(cmd.Env, "LD_LIBRARY_PATH="+d)
	}
}

// ---------- navigateur ----------

func trouverNavigateur() (string, string) {
	if cfg.Navigateur != "" && cfg.Navigateur != "auto" {
		if p, err := chemin(cfg.Navigateur); err == nil && existe(p) {
			return p, familleNom(filepath.Base(p))
		}
		if p, err := exec.LookPath(cfg.Navigateur); err == nil {
			return p, familleNom(filepath.Base(p))
		}
	}
	for _, n := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable",
		"brave-browser", "microsoft-edge", "microsoft-edge-stable"} {
		if p, err := exec.LookPath(n); err == nil {
			return p, "chromium"
		}
	}
	if p, err := exec.LookPath("firefox"); err == nil {
		return p, "firefox"
	}
	return "", ""
}

func familleNom(n string) string {
	if len(n) >= 7 && n[:7] == "firefox" {
		return "firefox"
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
		_ = navCmd.Process.Signal(syscall.SIGTERM)
	}
}

// fermerDoucement : SIGTERM, Hatari (SDL) le traite comme une fermeture de fenêtre.
func fermerDoucement(pid int) { _ = syscall.Kill(pid, syscall.SIGTERM) }

func ouvrirParDefaut(url string) { _ = exec.Command("xdg-open", url).Start() }

func premierPlan(pid int) {}

func revenirNavigateur() {
	if p, err := exec.LookPath("xdotool"); err == nil {
		time.Sleep(300 * time.Millisecond)
		_ = exec.Command(p, "search", "--name", "Ludothèque ST", "windowactivate").Run()
	}
}

// ---------- clavier (X11, sans dépendance de compilation) ----------

type clavierX11 struct {
	dpy      uintptr
	keymap   func(dpy uintptr, keys *[32]byte) int32
	pointer  func(dpy uintptr, w uint64, r, c *uint64, rx, ry, wx, wy *int32, m *uint32) int32
	keycodeP int
	root     uint64
	keycodeE int
	keycodeS int // Espace (Play Demo Random : démo suivante)
	px, py   int32
	touches  [32]byte
}

func ouvrirX11() *clavierX11 {
	if os.Getenv("DISPLAY") == "" {
		return nil
	}
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		log.Printf("clavier : libX11 absente (%v), Échap maintenu inactif", err)
		return nil
	}
	var open func(name uintptr) uintptr
	var k2c func(dpy uintptr, sym uint64) uint8
	c := &clavierX11{}
	purego.RegisterLibFunc(&open, lib, "XOpenDisplay")
	purego.RegisterLibFunc(&k2c, lib, "XKeysymToKeycode")
	purego.RegisterLibFunc(&c.keymap, lib, "XQueryKeymap")
	purego.RegisterLibFunc(&c.pointer, lib, "XQueryPointer")
	var rootw func(uintptr) uint64
	purego.RegisterLibFunc(&rootw, lib, "XDefaultRootWindow")
	c.dpy = open(0)
	if c.dpy == 0 {
		return nil
	}
	c.root = rootw(c.dpy)
	c.keycodeE = int(k2c(c.dpy, 0xff1b)) // XK_Escape
	c.keycodeP = int(k2c(c.dpy, 0xff13)) // XK_Pause
	c.keycodeS = int(k2c(c.dpy, 0x20))   // XK_space
	return c
}

// lire : état d'Échap, et vrai si la souris a bougé ou si une touche a changé.
func (c *clavierX11) pause() bool {
	if c == nil || c.keycodeP == 0 {
		return false
	}
	return c.touches[c.keycodeP/8]&(1<<(c.keycodeP%8)) != 0
}

func (c *clavierX11) espace() bool {
	if c == nil || c.keycodeS == 0 {
		return false
	}
	return c.touches[c.keycodeS/8]&(1<<(c.keycodeS%8)) != 0
}

func (c *clavierX11) lire() (echap, activite bool) {
	if c == nil || c.dpy == 0 {
		return false, false
	}
	var k [32]byte
	c.keymap(c.dpy, &k)
	if c.keycodeE != 0 {
		echap = k[c.keycodeE/8]&(1<<(c.keycodeE%8)) != 0
	}
	if k != c.touches {
		activite = true
		c.touches = k
	}
	var r, ch uint64
	var rx, ry, wx, wy int32
	var m uint32
	if c.pointer(c.dpy, c.root, &r, &ch, &rx, &ry, &wx, &wy, &m) != 0 {
		if rx != c.px || ry != c.py || m&0x1f00 != 0 {
			activite = true
		}
		c.px, c.py = rx, ry
	}
	return
}

// ---------- outils communs ----------

func compterManettes() int {
	m, _ := filepath.Glob("/dev/input/js*")
	return len(m)
}

// envoyerTouche : Linux passe par le tube de commandes de Hatari (scancode ST).
func envoyerTouche(fifo string, st, pc, pid int) {
	if fifo == "" {
		return
	}
	for _, ev := range []string{"keydown", "keyup"} {
		for essai := 0; essai < 20; essai++ {
			f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
			if err == nil {
				_, _ = f.WriteString("hatari-event " + ev + " 0x" + strconv.FormatInt(int64(st), 16) + "\n")
				f.Close()
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func ecrireFifo(fifo, ligne string) {
	if fifo == "" {
		return
	}
	for essai := 0; essai < 20; essai++ {
		f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_, _ = f.WriteString(ligne + "\n")
			f.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func attendreFenetre(pid int) {}

func finTurbo(fifo string, pid int) { ecrireFifo(fifo, "hatari-option --fast-forward no") }

// ---------- manettes (/dev/input/js*) ----------

type manetteJS struct {
	f       *os.File
	boutons map[int]bool
}

func ouvrirManettes() []*manetteJS {
	var out []*manetteJS
	for i := 0; i < 4; i++ {
		f, err := os.OpenFile("/dev/input/js"+string(rune('0'+i)), os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			continue
		}
		out = append(out, &manetteJS{f: f, boutons: map[int]bool{}})
	}
	return out
}

func (m *manetteJS) lire() bool {
	actif := false
	buf := make([]byte, 8*64)
	for {
		n, err := m.f.Read(buf)
		if n <= 0 || err != nil {
			return actif
		}
		for o := 0; o+8 <= n; o += 8 {
			val := int16(binary.LittleEndian.Uint16(buf[o+4:]))
			typ := buf[o+6] &^ 0x80
			num := int(buf[o+7])
			if typ == 1 {
				m.boutons[num] = val != 0
				if buf[o+6]&0x80 == 0 {
					actif = true
				}
			}
			if typ == 2 && buf[o+6]&0x80 == 0 && (val > 16000 || val < -16000) {
				actif = true
			}
		}
	}
}

func surveillerEntrees() {
	clavier := ouvrirX11()
	manettes := ouvrirManettes()
	dernierScan := time.Now()
	var echap, combo time.Duration
	var suivi suiviTouches
	var lecEsc, lecEsp bool
	const pas = 50 * time.Millisecond
	for range time.Tick(pas) {
		if time.Since(dernierScan) > 10*time.Second { // manette branchée en cours de route
			for _, m := range manettes {
				m.f.Close()
			}
			manettes = ouvrirManettes()
			dernierScan = time.Now()
		}
		for _, m := range manettes {
			if m.lire() {
				signalerActivite()
			}
		}
		esc, act := clavier.lire()
		if act {
			signalerActivite()
		}
		if jeux.EnJeu() == "demo" && demoEnLecture() {
			// Play Demo Random : Échap ou Select + Start pour revenir, Espace ou tir pour la suivante
			esp, ss := clavier.espace(), false
			for _, m := range manettes {
				esp = esp || m.boutons[0]
				for _, p := range cfg.BoutonsManette {
					ss = ss || (m.boutons[p[0]] && m.boutons[p[1]])
				}
			}
			if esc && !lecEsc {
				arreterDemos("Échap")
			} else if ss {
				arreterDemos("Select+Start")
			} else if esp && !lecEsp {
				demoSuivante()
			}
			lecEsc, lecEsp = esc, esp
			echap, combo = 0, 0
			continue
		}
		lecEsc, lecEsp = true, true // un appui en cours au lancement ne compte pas
		if e := jeux.EnJeu(); e == "libre" || e == "demo" {
			echap, combo = 0, 0
			continue
		}
		sel, start := false, false
		for _, m := range manettes {
			for _, p := range cfg.BoutonsManette {
				sel = sel || m.boutons[p[0]]
				start = start || m.boutons[p[1]]
			}
		}
		suivi.maj(clavier.pause(), sel, start)
		if esc {
			echap += pas
		} else {
			echap = 0
		}
		appui := false
		for _, m := range manettes {
			for _, p := range cfg.BoutonsManette {
				if m.boutons[p[0]] && m.boutons[p[1]] {
					appui = true
				}
			}
		}
		if appui {
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

func unsafePtr(a uintptr) unsafe.Pointer { return unsafe.Pointer(a) } //nolint
