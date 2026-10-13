//go:build linux

package main

import (
	"log"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ebitengine/purego"
)

// Bandeau des démos sous X11 : fenêtre « override-redirect » (hors gestionnaire
// de fenêtres), relevée régulièrement au-dessus de Hatari.

var (
	muBandeau   sync.Mutex
	stopBandeau chan struct{}
)

type x11 struct {
	open        func(uintptr) uintptr
	close       func(uintptr) int32
	defScreen   func(uintptr) int32
	root        func(uintptr, int32) uint64
	width       func(uintptr, int32) int32
	height      func(uintptr, int32) int32
	textWidth   func(uintptr, *byte, int32) int32
	black       func(uintptr, int32) uint64
	white       func(uintptr, int32) uint64
	create      func(uintptr, uint64, int32, int32, uint32, uint32, uint32, uint64, uint64) uint64
	changeAttrs func(uintptr, uint64, uint64, *[112]byte) int32
	mapRaised   func(uintptr, uint64) int32
	raise       func(uintptr, uint64) int32
	destroy     func(uintptr, uint64) int32
	createGC    func(uintptr, uint64, uint64, uintptr) uintptr
	setFg       func(uintptr, uintptr, uint64) int32
	loadFont    func(uintptr, *byte) uintptr
	setFont     func(uintptr, uintptr, uint64) int32
	draw        func(uintptr, uint64, uintptr, int32, int32, *byte, int32) int32
	fill        func(uintptr, uint64, uintptr, int32, int32, uint32, uint32) int32
	clear       func(uintptr, uint64) int32
	flush       func(uintptr) int32
}

var xlib *x11

func chargerX11() *x11 {
	if xlib != nil {
		return xlib
	}
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil
	}
	x := &x11{}
	reg := func(f any, n string) { purego.RegisterLibFunc(f, lib, n) }
	reg(&x.open, "XOpenDisplay")
	reg(&x.close, "XCloseDisplay")
	reg(&x.defScreen, "XDefaultScreen")
	reg(&x.root, "XRootWindow")
	reg(&x.width, "XDisplayWidth")
	reg(&x.height, "XDisplayHeight")
	reg(&x.textWidth, "XTextWidth")
	reg(&x.black, "XBlackPixel")
	reg(&x.white, "XWhitePixel")
	reg(&x.create, "XCreateSimpleWindow")
	reg(&x.changeAttrs, "XChangeWindowAttributes")
	reg(&x.mapRaised, "XMapRaised")
	reg(&x.raise, "XRaiseWindow")
	reg(&x.destroy, "XDestroyWindow")
	reg(&x.createGC, "XCreateGC")
	reg(&x.setFg, "XSetForeground")
	reg(&x.loadFont, "XLoadQueryFont")
	reg(&x.setFont, "XSetFont")
	reg(&x.draw, "XDrawString")
	reg(&x.fill, "XFillRectangle")
	reg(&x.clear, "XClearWindow")
	reg(&x.flush, "XFlush")
	xlib = x
	return x
}

// latin1 : les polices X de base sont en ISO-8859-1.
func latin1(s string) []byte {
	b := make([]byte, 0, len(s)+1)
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		s = s[n:]
		switch {
		case r < 256:
			b = append(b, byte(r))
		case r == '—' || r == '–':
			b = append(b, '-')
		case r == '’':
			b = append(b, '\'')
		default:
			b = append(b, '?')
		}
	}
	return append(b, 0)
}

func afficherCarte(c Carte) {
	masquerCarte()
	x := chargerX11()
	if x == nil {
		return
	}
	muBandeau.Lock()
	stop := make(chan struct{})
	stopBandeau = stop
	muBandeau.Unlock()
	go func() {
		dpy := x.open(0)
		if dpy == 0 {
			return
		}
		defer x.close(dpy)
		scr := x.defScreen(dpy)
		sw, sh := x.width(dpy, scr), x.height(dpy, scr)
		noir, blanc := x.black(dpy, scr), x.white(dpy, scr)
		w, h := sw, sh
		if c.Mode == "haut" {
			h = int32(40 + 24*len(c.Lignes) + 12)
		}
		win := x.create(dpy, x.root(dpy, scr), 0, 0, uint32(w), uint32(h), 0, noir, blanc)
		var attrs [112]byte
		attrs[88] = 1                         // override_redirect = True
		x.changeAttrs(dpy, win, 1<<9, &attrs) // CWOverrideRedirect
		x.mapRaised(dpy, win)
		gc := x.createGC(dpy, win, 0, 0)
		charger := func(noms ...string) (uint64, uintptr) {
			for _, n := range noms {
				if f := x.loadFont(dpy, &latin1(n)[0]); f != 0 {
					return *(*uint64)(unsafePtr(f + 8)), f
				}
			}
			return 0, 0
		}
		grande, fg := charger("-*-dejavu sans mono-bold-r-normal--28-*-*-*-*-*-iso8859-1", "-*-*-bold-r-normal--24-*-*-*-c-*-iso8859-1", "12x24", "10x20", "9x15bold", "fixed")
		petite, fp := charger("-*-dejavu sans mono-medium-r-normal--20-*-*-*-*-*-iso8859-1", "10x20", "9x15", "8x13", "fixed")
		largeur := func(f uintptr, t []byte) int32 {
			if f == 0 {
				return int32(len(t)-1) * 9
			}
			return x.textWidth(f, &t[0], int32(len(t)-1))
		}
		ecrire := func(font uint64, xx, yy int32, t []byte) {
			if font != 0 {
				x.setFont(dpy, gc, font)
			}
			x.draw(dpy, win, gc, xx, yy, &t[0], int32(len(t)-1))
		}
		dessiner := func() {
			switch c.Mode {
			case "haut":
				x.setFg(dpy, gc, blanc)
				x.fill(dpy, win, gc, 0, 0, uint32(w), uint32(h))
				x.setFg(dpy, gc, noir)
				x.fill(dpy, win, gc, 0, h-3, uint32(w), 3)
				ecrire(grande, 24, 30, latin1(c.Titre))
				for i, l := range c.Lignes {
					ecrire(petite, 24, int32(60+24*i), latin1(l))
				}
			default:
				x.setFg(dpy, gc, noir)
				x.fill(dpy, win, gc, 0, 0, uint32(w), uint32(h))
				pw, ph := int32(600), int32(70+30*len(c.Lignes)+40)
				if l := largeur(fg, latin1(c.Titre)) + 60; l > pw {
					pw = l
				}
				for _, ligne := range c.Lignes {
					if l := largeur(fp, latin1(ligne)) + 60; l > pw {
						pw = l
					}
				}
				if pw > w-20 {
					pw = w - 20
				}
				x0, y0 := (w-pw)/2, (h-ph)/2
				x.setFg(dpy, gc, blanc)
				x.fill(dpy, win, gc, x0+4, y0+4, uint32(pw-8), uint32(ph-8))
				x.setFg(dpy, gc, noir)
				x.fill(dpy, win, gc, x0+9, y0+9, uint32(pw-18), uint32(ph-18))
				x.setFg(dpy, gc, blanc)
				x.fill(dpy, win, gc, x0+11, y0+11, uint32(pw-22), uint32(ph-22))
				x.setFg(dpy, gc, noir)
				t := latin1(c.Titre)
				ecrire(grande, x0+(pw-largeur(fg, t))/2, y0+50, t)
				for i, l := range c.Lignes {
					t := latin1(l)
					ecrire(petite, x0+(pw-largeur(fp, t))/2, y0+90+int32(30*i), t)
				}
			}
			x.raise(dpy, win)
			x.flush(dpy)
		}
		t := time.NewTicker(400 * time.Millisecond)
		defer t.Stop()
		dessiner()
		for {
			select {
			case <-stop:
				x.destroy(dpy, win)
				x.flush(dpy)
				return
			case <-t.C:
				dessiner()
			}
		}
	}()
	log.Printf("carte : %s", c.Titre)
}

func masquerCarte() {
	muBandeau.Lock()
	defer muBandeau.Unlock()
	if stopBandeau != nil {
		close(stopBandeau)
		stopBandeau = nil
	}
}

var fifoCourant string

func pauseHatari(pause bool) {
	if pause {
		ecrireFifo(fifoCourant, "hatari-stop")
	} else {
		ecrireFifo(fifoCourant, "hatari-cont")
	}
}

func revenirJeu() {}

func fifoCourantMaj(f string) { fifoCourant = f }
