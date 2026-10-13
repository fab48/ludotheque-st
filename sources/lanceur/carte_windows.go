//go:build windows

package main

import (
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Carte affichée par-dessus Hatari : fenêtre Win32 sans bordure, toujours au
// premier plan, qui ne prend pas le focus ; dessinée au GDI façon GEM.

var (
	gdi32             = syscall.NewLazyDLL("gdi32.dll")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	pCreateWindowEx   = user32.NewProc("CreateWindowExW")
	pDestroyWindow    = user32.NewProc("DestroyWindow")
	pSetWindowPos     = user32.NewProc("SetWindowPos")
	pPeekMessage      = user32.NewProc("PeekMessageW")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessage  = user32.NewProc("DispatchMessageW")
	pDefWindowProc    = user32.NewProc("DefWindowProcW")
	pRegisterClassEx  = user32.NewProc("RegisterClassExW")
	pBeginPaint       = user32.NewProc("BeginPaint")
	pEndPaint         = user32.NewProc("EndPaint")
	pGetClientRect    = user32.NewProc("GetClientRect")
	pFillRect         = user32.NewProc("FillRect")
	pDrawText         = user32.NewProc("DrawTextW")
	pInvalidateRect   = user32.NewProc("InvalidateRect")
	pGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	pGetLastInputInfo = user32.NewProc("GetLastInputInfo")
	pCreateFont       = gdi32.NewProc("CreateFontW")
	pDeleteObject     = gdi32.NewProc("DeleteObject")
	pCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	pSelectObject     = gdi32.NewProc("SelectObject")
	pSetTextColor     = gdi32.NewProc("SetTextColor")
	pSetBkMode        = gdi32.NewProc("SetBkMode")
	pGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")

	muFenetre    sync.Mutex
	stopFenetre  chan struct{}
	carteDessin  Carte
	classeOK     bool
	nomClasse, _ = syscall.UTF16PtrFromString("LudothequeCarte")
)

type msgW struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	X, Y    int32
	Private uint32
}

type rect struct{ L, T, R, B int32 }

type paintStruct struct {
	Hdc      uintptr
	Erase    int32
	Rc       rect
	Restore  int32
	IncUpd   int32
	Reserved [32]byte
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

func police(taille int, gras bool) uintptr {
	nom, _ := syscall.UTF16PtrFromString("Consolas")
	poids := 400
	if gras {
		poids = 700
	}
	f, _, _ := pCreateFont.Call(uintptr(uint32(int32(-taille))), 0, 0, 0, uintptr(poids), 0, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(nom)))
	return f
}

func texte(hdc uintptr, s string, r *rect, format uintptr) {
	p, _ := syscall.UTF16FromString(s)
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)-1), uintptr(unsafe.Pointer(r)), format)
}

var procCarte = syscall.NewCallback(func(h, msg, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0014: // WM_ERASEBKGND
		return 1
	case 0x000F: // WM_PAINT
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
		muFenetre.Lock()
		c := carteDessin
		muFenetre.Unlock()
		var rc rect
		pGetClientRect.Call(h, uintptr(unsafe.Pointer(&rc)))
		noir, _, _ := pCreateSolidBrush.Call(0x000000)
		blanc, _, _ := pCreateSolidBrush.Call(0xFFFFFF)
		pSetBkMode.Call(hdc, 1)
		pSetTextColor.Call(hdc, 0)
		hauteurEcran := rc.B
		if c.Mode != "haut" {
			hauteurEcran, _, _ := pGetSystemMetrics.Call(1)
			_ = hauteurEcran
		}
		const dtCenter, dtLeft, dtSingle, dtVCenter, dtNoPrefix = 0x1, 0x0, 0x20, 0x4, 0x800
		switch c.Mode {
		case "haut":
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), blanc)
			bas := rect{rc.L, rc.B - 3, rc.R, rc.B}
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&bas)), noir)
			fg, fp := police(26, true), police(20, false)
			pSelectObject.Call(hdc, fg)
			r := rect{24, 8, rc.R - 24, 40}
			texte(hdc, c.Titre, &r, dtLeft|dtSingle|dtVCenter|dtNoPrefix)
			pSelectObject.Call(hdc, fp)
			for i, l := range c.Lignes {
				r := rect{24, int32(42 + 24*i), rc.R - 24, int32(66 + 24*i)}
				texte(hdc, l, &r, dtLeft|dtSingle|dtVCenter|dtNoPrefix)
			}
			pDeleteObject.Call(fg)
			pDeleteObject.Call(fp)
		default:
			// panneau GEM centré (plein écran : fond noir autour)
			if c.Mode == "plein" {
				pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), noir)
			}
			w, h := rc.R-rc.L, rc.B-rc.T
			taille := int(h / 26)
			if c.Mode == "plein" {
				taille = int(h / 40)
			}
			if taille < 14 {
				taille = 14
			}
			longue := len([]rune(c.Titre)) * 3 / 2
			for _, l := range c.Lignes {
				if n := len([]rune(l)); n > longue {
					longue = n
				}
			}
			for taille > 14 && int32(taille*longue*56/100)+60 > w-40 {
				taille--
			}
			plus := len([]rune(c.Titre)) * 3 / 2
			for _, l := range c.Lignes {
				if n := len([]rune(l)); n > plus {
					plus = n
				}
			}
			// Consolas : un caractère ≈ 0,55 × la taille ; marges comprises
			pw := int32(taille*plus*56/100) + 60
			if pw < int32(taille)*24 {
				pw = int32(taille) * 24
			}
			if pw > w-20 {
				pw = w - 20
			}
			ph := int32(taille)*3 + int32(taille*13/10)*int32(len(c.Lignes)) + int32(taille)*3/2
			if ph > h-20 {
				ph = h - 20
			}
			x0, y0 := (w-pw)/2, (h-ph)/2
			cadre := rect{x0, y0, x0 + pw, y0 + ph}
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&cadre)), noir)
			int1 := rect{x0 + 4, y0 + 4, x0 + pw - 4, y0 + ph - 4}
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&int1)), blanc)
			int2 := rect{x0 + 9, y0 + 9, x0 + pw - 9, y0 + ph - 9}
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&int2)), noir)
			int3 := rect{x0 + 11, y0 + 11, x0 + pw - 11, y0 + ph - 11}
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&int3)), blanc)
			fg, fp := police(taille*3/2, true), police(taille, false)
			pSelectObject.Call(hdc, fg)
			y := y0 + int32(taille)
			r := rect{x0 + 16, y, x0 + pw - 16, y + int32(taille*2)}
			texte(hdc, c.Titre, &r, dtCenter|dtSingle|dtVCenter|dtNoPrefix)
			y += int32(taille * 2)
			pSelectObject.Call(hdc, fp)
			for _, l := range c.Lignes {
				r := rect{x0 + 16, y, x0 + pw - 16, y + int32(taille*13/10)}
				texte(hdc, l, &r, dtCenter|dtSingle|dtVCenter|dtNoPrefix)
				y += int32(taille * 13 / 10)
			}
			pDeleteObject.Call(fg)
			pDeleteObject.Call(fp)
		}
		_ = hauteurEcran
		pDeleteObject.Call(noir)
		pDeleteObject.Call(blanc)
		pEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := pDefWindowProc.Call(h, msg, wp, lp)
	return r
})

func afficherCarte(c Carte) {
	masquerCarte()
	muFenetre.Lock()
	carteDessin = c
	stop := make(chan struct{})
	stopFenetre = stop
	muFenetre.Unlock()
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		inst, _, _ := pGetModuleHandle.Call(0)
		if !classeOK {
			wc := wndClassEx{WndProc: procCarte, Instance: inst, ClassName: nomClasse}
			wc.Size = uint32(unsafe.Sizeof(wc))
			pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
			classeOK = true
		}
		sw, _, _ := pGetSystemMetrics.Call(0)
		sh, _, _ := pGetSystemMetrics.Call(1)
		x, y, w, h := uintptr(0), uintptr(0), sw, sh
		if c.Mode == "haut" {
			h = uintptr(40 + 24*len(c.Lignes) + 12)
		}
		const (
			wsPopup, wsVisible          = 0x80000000, 0x10000000
			wsExTopmost, wsExToolwindow = 0x8, 0x80
			wsExNoActivate              = 0x08000000
			wsExTransparent             = 0x20
			hwndTopmost                 = ^uintptr(0)
			swpNoActivate, swpShow      = 0x10, 0x40
		)
		ex := uintptr(wsExTopmost | wsExToolwindow | wsExNoActivate)
		if c.Mode == "centre" { // clics transmis au jeu
			ex |= wsExTransparent
		}
		titre, _ := syscall.UTF16PtrFromString("Ludothèque ST - carte")
		hw, _, _ := pCreateWindowEx.Call(ex, uintptr(unsafe.Pointer(nomClasse)), uintptr(unsafe.Pointer(titre)),
			wsPopup|wsVisible, x, y, w, h, 0, 0, inst, 0)
		if hw == 0 {
			return
		}
		var m msgW
		tic := time.NewTicker(40 * time.Millisecond)
		defer tic.Stop()
		n := 0
		for {
			select {
			case <-stop:
				pDestroyWindow.Call(hw)
				return
			case <-tic.C:
				for {
					r, _, _ := pPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
					if r == 0 {
						break
					}
					pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
					pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
				}
				if n++; n%10 == 1 { // rester au-dessus de Hatari en plein écran
					pSetWindowPos.Call(hw, hwndTopmost, x, y, w, h, swpNoActivate|swpShow)
					pInvalidateRect.Call(hw, 0, 1)
				}
			}
		}
	}()
}

func masquerCarte() {
	muFenetre.Lock()
	defer muFenetre.Unlock()
	if stopFenetre != nil {
		close(stopFenetre)
		stopFenetre = nil
	}
}

// pauseHatari : touche Pause envoyée à Hatari (raccourci « pause »).
func pauseHatari(bool) {
	jeux.mu.Lock()
	cmd := jeux.cmd
	jeux.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if hs := fenetresDuProcessus(cmd.Process.Pid); len(hs) > 0 {
		forcerPremierPlan(hs[0])
		time.Sleep(60 * time.Millisecond)
		pKeybdEvent.Call(0x13, 0x45, 0, 0)
		time.Sleep(80 * time.Millisecond)
		pKeybdEvent.Call(0x13, 0x45, 2, 0)
	}
}

func revenirJeu() {
	jeux.mu.Lock()
	cmd := jeux.cmd
	jeux.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		if hs := fenetresDuProcessus(cmd.Process.Pid); len(hs) > 0 {
			forcerPremierPlan(hs[0])
		}
	}
}

type lastInputInfo struct{ Size, Time uint32 }

var dernierTick uint32

// activiteSysteme : vrai si le clavier ou la souris ont servi depuis le dernier appel.
func activiteSysteme() bool {
	li := lastInputInfo{Size: 8}
	if r, _, _ := pGetLastInputInfo.Call(uintptr(unsafe.Pointer(&li))); r == 0 {
		return false
	}
	change := dernierTick != 0 && li.Time != dernierTick
	dernierTick = li.Time
	return change
}

var _ = strings.TrimSpace

func fifoCourantMaj(string) {}
