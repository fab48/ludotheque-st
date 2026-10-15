//go:build windows

package main

import (
	"fmt"
	"unsafe"
)

var (
	pEnumDisplayDevices  = user32.NewProc("EnumDisplayDevicesW")
	pEnumDisplaySettings = user32.NewProc("EnumDisplaySettingsW")
	pChangeDisplay       = user32.NewProc("ChangeDisplaySettingsExW")
)

// devMode : DEVMODEW (220 octets), partie « écran ».
type devMode struct {
	DeviceName         [32]uint16
	SpecVersion        uint16
	DriverVersion      uint16
	Size               uint16
	DriverExtra        uint16
	Fields             uint32
	PositionX          int32
	PositionY          int32
	DisplayOrientation uint32
	DisplayFixedOutput uint32
	Color              int16
	Duplex             int16
	YResolution        int16
	TTOption           int16
	Collate            int16
	FormName           [32]uint16
	LogPixels          uint16
	BitsPerPel         uint32
	PelsWidth          uint32
	PelsHeight         uint32
	DisplayFlags       uint32
	DisplayFrequency   uint32
	ICMMethod          uint32
	ICMIntent          uint32
	MediaType          uint32
	DitherType         uint32
	Reserved1          uint32
	Reserved2          uint32
	PanningWidth       uint32
	PanningHeight      uint32
}

var _ = [1]struct{}{}[unsafe.Sizeof(devMode{})-220] // taille de DEVMODEW vérifiée à la compilation

type displayDevice struct {
	Cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

const (
	enumCurrentSettings = 0xFFFFFFFF
	dmInterlaced        = 0x2
	dmPelsWidth         = 0x80000
	dmPelsHeight        = 0x100000
	dmDisplayFlags      = 0x200000
	dmDisplayFrequency  = 0x400000
	cdsFullscreen       = 0x4 // temporaire : pas écrit dans le registre
	ddPrimaryDevice     = 0x4
)

var ecranPrincipal *uint16 // nom du périphérique (\\.\DISPLAYn), nil = défaut

// trouverEcranPrincipal : l'écran où Hatari s'ouvre en plein écran.
func trouverEcranPrincipal() *uint16 {
	for i := uint32(0); ; i++ {
		var d displayDevice
		d.Cb = uint32(unsafe.Sizeof(d))
		if r, _, _ := pEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(&d)), 0); r == 0 {
			return nil
		}
		if d.StateFlags&ddPrimaryDevice != 0 {
			n := d.DeviceName
			return &n[0]
		}
	}
}

func modeEcran(n uint32, m *devMode) bool {
	m.Size = uint16(unsafe.Sizeof(*m))
	r, _, _ := pEnumDisplaySettings.Call(uintptr(unsafe.Pointer(ecranPrincipal)), uintptr(n), uintptr(unsafe.Pointer(m)))
	return r != 0
}

// passerFrequence : écran principal à hz (ou 2×hz), même définition, non entrelacé.
func passerFrequence(hz int) (avant, apres int, err error) {
	ecranPrincipal = trouverEcranPrincipal()
	var cur devMode
	if !modeEcran(enumCurrentSettings, &cur) {
		return 0, 0, fmt.Errorf("mode actuel illisible")
	}
	avant = int(cur.DisplayFrequency)
	possibles := map[int]bool{}
	for i := uint32(0); ; i++ {
		var m devMode
		if !modeEcran(i, &m) {
			break
		}
		if m.PelsWidth == cur.PelsWidth && m.PelsHeight == cur.PelsHeight && m.DisplayFlags&dmInterlaced == 0 {
			possibles[int(m.DisplayFrequency)] = true
		}
	}
	for _, f := range []int{hz, 2 * hz} {
		if f == avant {
			return avant, avant, nil
		}
	}
	for _, f := range []int{hz, 2 * hz} {
		if !possibles[f] {
			continue
		}
		m := cur
		m.Fields = dmPelsWidth | dmPelsHeight | dmDisplayFlags | dmDisplayFrequency
		m.DisplayFlags = 0
		m.DisplayFrequency = uint32(f)
		r, _, _ := pChangeDisplay.Call(uintptr(unsafe.Pointer(ecranPrincipal)), uintptr(unsafe.Pointer(&m)), 0, cdsFullscreen, 0)
		if int32(r) != 0 {
			return avant, avant, fmt.Errorf("%d Hz refusé par le pilote (code %d)", f, int32(r))
		}
		return avant, f, nil
	}
	return avant, avant, fmt.Errorf("l'écran (%d×%d) ne propose ni %d ni %d Hz", cur.PelsWidth, cur.PelsHeight, hz, 2*hz)
}

// remettreFrequence : retour au mode enregistré dans Windows.
func remettreFrequence() error {
	r, _, _ := pChangeDisplay.Call(uintptr(unsafe.Pointer(ecranPrincipal)), 0, 0, 0, 0)
	if int32(r) != 0 {
		return fmt.Errorf("code %d", int32(r))
	}
	return nil
}
